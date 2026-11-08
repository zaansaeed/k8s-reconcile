package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// maxDeployments caps the output so a crowded namespace can't flood the
// model's context; diagnosis scenarios have one or two.
const maxDeployments = 10

// GetDeployments returns every deployment in the namespace as compact text:
// replica counts, rollout conditions and the pod template. This is the desired
// state the model compares against what get_pods and get_events report.
func (t *Toolbox) GetDeployments(ctx context.Context) (string, error) {
	deps, err := t.client.AppsV1().Deployments(t.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing deployments in %s: %w", t.namespace, err)
	}
	if len(deps.Items) == 0 {
		return fmt.Sprintf("no deployments in namespace %s\n", t.namespace), nil
	}

	items := deps.Items
	if len(items) > maxDeployments {
		items = items[:maxDeployments]
	}

	var b strings.Builder
	for i, deployment := range items { // iterate over all deployments in the namespace
		if i > 0 {
			b.WriteString("\n")
		}
		writeDeployment(&b, deployment)
	}
	return b.String(), nil
}

// desiredReplicas returns how many pods a deployment asks for. Kubernetes
// treats an unset replicas field as 1, so a nil pointer means 1.
func desiredReplicas(d *appsv1.Deployment) int32 {
	if d.Spec.Replicas == nil {
		return 1
	}
	return *d.Spec.Replicas
}

// writeDeployment writes one deployment: replicas and conditions first, since
// they say whether it is healthy, then its containers and volumes.
func writeDeployment(b *strings.Builder, d appsv1.Deployment) {
	desired := desiredReplicas(&d)
	fmt.Fprintf(b, "deployment/%s  replicas: desired=%d updated=%d ready=%d available=%d\n",
		d.Name, desired, d.Status.UpdatedReplicas, d.Status.ReadyReplicas, d.Status.AvailableReplicas)

	for _, c := range d.Status.Conditions { // controller's verdict: Available, Progressing
		fmt.Fprintf(b, "  condition %s=%s", c.Type, c.Status)
		if c.Reason != "" {
			fmt.Fprintf(b, " reason=%s", c.Reason)
		}
		if c.Message != "" {
			fmt.Fprintf(b, " message=%q", c.Message)
		}
		b.WriteString("\n")
	}

	spec := d.Spec.Template.Spec
	for _, c := range spec.InitContainers { // run one at a time, each must exit 0 first
		writeContainer(b, "init container", c)
	}
	for _, c := range spec.Containers { // the app; start once all init containers finish
		writeContainer(b, "container", c)
	}
	for _, v := range spec.Volumes { // defined at pod level, mounted by containers
		if s := volumeSource(v); s != "" {
			fmt.Fprintf(b, "  volume %s: %s\n", v.Name, s)
		}
	}
}

// writeContainer writes the container settings that commonly cause faults.
// It prints "env: none" explicitly, because a missing variable is itself a cause.
func writeContainer(b *strings.Builder, label string, c corev1.Container) {
	fmt.Fprintf(b, "  %s %s\n", label, c.Name)
	fmt.Fprintf(b, "    image: %s\n", c.Image)
	if len(c.Command) > 0 { // replaces the image's default program
		fmt.Fprintf(b, "    command: %q\n", c.Command)
	}
	if len(c.Args) > 0 { // arguments passed to that program
		fmt.Fprintf(b, "    args: %q\n", c.Args)
	}
	for _, p := range c.Ports { // ports the container listens on
		fmt.Fprintf(b, "    port: %d/%s\n", p.ContainerPort, p.Protocol)
	}

	if len(c.Env) == 0 && len(c.EnvFrom) == 0 { // say so when there are none at all
		b.WriteString("    env: none\n")
	}
	for _, e := range c.Env { // variables set one by one, as a value or a reference
		fmt.Fprintf(b, "    env %s: %s\n", e.Name, envSource(e))
	}
	for _, f := range c.EnvFrom { // imports every key of a configmap or secret at once
		switch {
		case f.ConfigMapRef != nil:
			fmt.Fprintf(b, "    envFrom: all keys of configmap/%s\n", f.ConfigMapRef.Name)
		case f.SecretRef != nil:
			fmt.Fprintf(b, "    envFrom: all keys of secret/%s\n", f.SecretRef.Name)
		}
	}

	for _, m := range c.VolumeMounts { // where each pod volume appears inside the container
		fmt.Fprintf(b, "    mount: volume %s at %s\n", m.Name, m.MountPath)
	}

	if p := c.LivenessProbe; p != nil { // failing it gets the container restarted
		fmt.Fprintf(b, "    liveness probe: %s\n", probe(p))
	}
	if p := c.ReadinessProbe; p != nil { // failing it marks the pod not ready
		fmt.Fprintf(b, "    readiness probe: %s\n", probe(p))
	}

	if r := resources(c.Resources); r != "" {
		fmt.Fprintf(b, "    resources: %s\n", r)
	}
}

// envSource says where an env var's value comes from. Secret-backed values are
// named but never read, so secret contents can't reach the model.
func envSource(e corev1.EnvVar) string {
	if e.ValueFrom == nil {
		return fmt.Sprintf("value %q", e.Value)
	}
	switch v := e.ValueFrom; {
	case v.SecretKeyRef != nil:
		return fmt.Sprintf("from secret/%s key %s (value not shown)", v.SecretKeyRef.Name, v.SecretKeyRef.Key)
	case v.ConfigMapKeyRef != nil:
		return fmt.Sprintf("from configmap/%s key %s", v.ConfigMapKeyRef.Name, v.ConfigMapKeyRef.Key)
	case v.FieldRef != nil:
		return fmt.Sprintf("from field %s", v.FieldRef.FieldPath)
	case v.ResourceFieldRef != nil:
		return fmt.Sprintf("from resource %s", v.ResourceFieldRef.Resource)
	default:
		return "from an unrecognized source"
	}
}

// volumeSource names the object behind a volume, so the model can check that
// it exists. Kinds that rarely cause faults return "" and are skipped.
func volumeSource(v corev1.Volume) string {
	switch {
	case v.ConfigMap != nil:
		return "configmap/" + v.ConfigMap.Name
	case v.Secret != nil:
		return "secret/" + v.Secret.SecretName
	case v.PersistentVolumeClaim != nil:
		return "persistentvolumeclaim/" + v.PersistentVolumeClaim.ClaimName
	case v.EmptyDir != nil:
		return "emptyDir"
	default:
		return ""
	}
}

// probe describes a health check: what it hits and how often. A wrong path or
// port here makes a healthy container look broken.
func probe(p *corev1.Probe) string {
	var target string
	switch {
	case p.HTTPGet != nil:
		target = fmt.Sprintf("http GET %s on port %s", p.HTTPGet.Path, p.HTTPGet.Port.String())
	case p.TCPSocket != nil:
		target = fmt.Sprintf("tcp on port %s", p.TCPSocket.Port.String())
	case p.Exec != nil:
		target = fmt.Sprintf("exec %q", p.Exec.Command)
	case p.GRPC != nil:
		target = fmt.Sprintf("grpc on port %d", p.GRPC.Port)
	default:
		target = "unknown handler"
	}
	return fmt.Sprintf("%s, initialDelay=%ds period=%ds failureThreshold=%d",
		target, p.InitialDelaySeconds, p.PeriodSeconds, p.FailureThreshold)
}

// resources lists CPU and memory requests, which decide whether the pod can be
// scheduled, and limits, which decide when it gets killed.
func resources(r corev1.ResourceRequirements) string {
	var parts []string
	parts = append(parts, quantities("request", r.Requests)...)
	parts = append(parts, quantities("limit", r.Limits)...)
	return strings.Join(parts, " ")
}

// quantities formats a resource map in sorted order. Go randomizes map
// iteration, and unsorted output would differ between identical runs.
func quantities(label string, list corev1.ResourceList) []string {
	names := make([]string, 0, len(list))
	for name := range list {
		names = append(names, string(name))
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		q := list[corev1.ResourceName(name)]
		parts = append(parts, fmt.Sprintf("%s %s=%s", label, name, q.String()))
	}
	return parts
}
