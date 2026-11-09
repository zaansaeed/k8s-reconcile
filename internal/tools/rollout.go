package tools

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rolloutWait is how long a patch waits for its rollout before reporting it
// unfinished; failing pods end the wait early, so it is only reached when the
// new pods are slow or never become ready. rolloutPoll is how often the
// deployment is re-read, and rolloutSettle is how long the new pods must stay
// up before a rollout counts as complete.
const (
	rolloutWait   = 60 * time.Second
	rolloutPoll   = 2 * time.Second
	rolloutSettle = 5 * time.Second
)

// waitForRollout polls a deployment after a patch until its new pods have
// stayed up for the settling period, a new pod is seen failing, or the wait
// ends, then reports the verdict followed by every pod in the namespace. This
// is how the agent learns whether a fix worked, or exposed a fault the first
// one was hiding.
func (t *Toolbox) waitForRollout(ctx context.Context, name string) (string, error) {
	deadline := time.Now().Add(rolloutWait)
	var stableSince time.Time
	for {
		d, err := t.client.AppsV1().Deployments(t.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("reading deployment %s: %w", name, err)
		}

		complete, detail := rolloutStatus(d)
		if d.Status.ObservedGeneration >= d.Generation { // before this the new pods don't exist yet
			problem, err := t.unstablePod(ctx, d, complete)
			if err != nil {
				return "", err
			}
			if problem != "" { // deployed is not the same as staying up; stop waiting on pods that are failing
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s deployed but not stable: %s.", name, problem))
			}
		}
		if complete {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= rolloutSettle {
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s complete: %s, and stayed up without restarting for %s.", name, detail, rolloutSettle))
			}
		} else {
			stableSince = time.Time{}
			if !time.Now().Before(deadline) {
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s not finished after %s: %s.", name, rolloutWait, detail))
			}
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(rolloutPoll):
		}
	}
}

// rolloutStatus applies the same checks as kubectl rollout status. The
// generation check comes first because right after a patch the status still
// describes the old version and would wrongly read as complete.
func rolloutStatus(d *appsv1.Deployment) (bool, string) {
	status := d.Status
	if status.ObservedGeneration < d.Generation {
		return false, "the controller has not processed the patch yet"
	}
	desired := desiredReplicas(d)
	complete := status.UpdatedReplicas >= desired && status.Replicas == status.UpdatedReplicas && status.AvailableReplicas >= desired
	return complete, fmt.Sprintf("%d of %d pods run the new template, %d available, %d from the old template still running",
		status.UpdatedReplicas, desired, status.AvailableReplicas, status.Replicas-status.UpdatedReplicas)
}

// startingUp lists the waiting reasons a healthy new pod passes through, which
// must not be mistaken for a failure mid-rollout.
var startingUp = map[string]bool{"ContainerCreating": true, "PodInitializing": true}

// unstablePod looks for a pod of the deployment that is failing: it has
// restarted, has exited, or is waiting for a reason other than starting up.
// Without a readiness probe, a crash-looping container counts as available in
// the moments it runs, so rollout status alone can't catch it. Only pods from
// the current template count, because mid-rollout the old pods may still show
// the fault being fixed. Mid-rollout, if the current template's pods can't be
// identified yet, nothing is checked. It returns "" when every pod looks stable.
func (t *Toolbox) unstablePod(ctx context.Context, d *appsv1.Deployment, complete bool) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("reading selector of deployment %s: %w", d.Name, err)
	}
	hash, err := t.currentTemplateHash(ctx, d, selector.String())
	if err != nil {
		return "", err
	}
	if hash == "" && !complete {
		return "", nil
	}
	pods, err := t.client.CoreV1().Pods(t.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", fmt.Errorf("listing pods of deployment %s: %w", d.Name, err)
	}

	for _, p := range pods.Items {
		if p.DeletionTimestamp != nil || p.Status.Phase == corev1.PodSucceeded { // old pods on their way out
			continue
		}
		if hash != "" && p.Labels[appsv1.DefaultDeploymentUniqueLabelKey] != hash { // from an older template
			continue
		}
		for _, c := range p.Status.ContainerStatuses {
			switch {
			case c.State.Waiting != nil && !startingUp[c.State.Waiting.Reason]:
				return fmt.Sprintf("pod %s container %s is waiting: %s", p.Name, c.Name, c.State.Waiting.Reason), nil
			case c.State.Terminated != nil:
				return fmt.Sprintf("pod %s container %s has exited: %s (exit %d)", p.Name, c.Name, c.State.Terminated.Reason, c.State.Terminated.ExitCode), nil
			case c.RestartCount > 0:
				return fmt.Sprintf("pod %s container %s has restarted %d times", p.Name, c.Name, c.RestartCount), nil
			}
		}
	}
	return "", nil
}

// revisionAnnotation is set by the deployment controller on a Deployment and on
// each of its ReplicaSets. The ReplicaSet whose revision matches the
// Deployment's is the one running the current template.
const revisionAnnotation = "deployment.kubernetes.io/revision"

// currentTemplateHash returns the pod-template-hash label of the ReplicaSet
// running the deployment's current template, which every pod it created
// carries. It returns "" if the controller hasn't created that ReplicaSet yet.
func (t *Toolbox) currentTemplateHash(ctx context.Context, d *appsv1.Deployment, selector string) (string, error) {
	revision := d.Annotations[revisionAnnotation]
	if revision == "" {
		return "", nil
	}
	sets, err := t.client.AppsV1().ReplicaSets(t.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("listing replicasets of deployment %s: %w", d.Name, err)
	}
	for _, rs := range sets.Items {
		if metav1.IsControlledBy(&rs, d) && rs.Annotations[revisionAnnotation] == revision {
			return rs.Labels[appsv1.DefaultDeploymentUniqueLabelKey], nil
		}
	}
	return "", nil
}

// rolloutReport follows the verdict's headline with every pod in the
// namespace, since a fix to one deployment can leave another still broken.
func (t *Toolbox) rolloutReport(ctx context.Context, headline string) (string, error) {
	pods, err := t.GetPods(ctx)
	if err != nil {
		return "", err
	}
	return headline + "\npods now:\n" + pods, nil
}
