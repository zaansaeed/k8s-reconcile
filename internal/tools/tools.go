// Package tools exposes the Kubernetes operations the agent may call,
// scoped to a single namespace and formatted as compact text for the LLM.
package tools

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Tools gives the agent access to one namespace. The namespace is fixed at
// construction so the LLM can never point a tool at another namespace.
type Tools struct {
	client    kubernetes.Interface
	namespace string
}

// New returns Tools scoped to namespace.
func New(client kubernetes.Interface, namespace string) *Tools {
	return &Tools{client: client, namespace: namespace}
}

// GetPods returns one line per pod with phase, readiness, restarts and, when a
// container is unhealthy, the reason. Phase alone hides most faults
// (ImagePullBackOff is still "Pending"), so container details are included.
func (t *Tools) GetPods(ctx context.Context) (string, error) {
	pods, err := t.client.CoreV1().Pods(t.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing pods in %s: %w", t.namespace, err)
	}
	if len(pods.Items) == 0 {
		// An empty result is ambiguous to an LLM; say so explicitly.
		return fmt.Sprintf("no pods in namespace %s\n", t.namespace), nil
	}

	var b strings.Builder
	for _, p := range pods.Items {
		fmt.Fprintf(&b, "%s  %s  ready=%s  restarts=%d", p.Name, p.Status.Phase, readyCount(p), restarts(p))
		if r := problems(p); r != "" {
			fmt.Fprintf(&b, "  %s", r)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// readyCount returns "ready/total" for the pod's containers, like kubectl's READY column.
func readyCount(p corev1.Pod) string {
	ready := 0
	for _, c := range p.Status.ContainerStatuses {
		if c.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, len(p.Spec.Containers))
}

func restarts(p corev1.Pod) int32 {
	var n int32
	for _, c := range p.Status.ContainerStatuses {
		n += c.RestartCount
	}
	return n
}

// problems summarizes why containers are unhealthy, e.g.
// "nginx waiting: ImagePullBackOff" or "stress last: OOMKilled".
func problems(p corev1.Pod) string {
	var parts []string
	for _, c := range p.Status.ContainerStatuses {
		// State.Waiting and LastTerminationState.Terminated are pointers that
		// are nil unless that state applies, so check before reading them.
		if w := c.State.Waiting; w != nil {
			parts = append(parts, fmt.Sprintf("%s waiting: %s", c.Name, w.Reason))
		}
		if last := c.LastTerminationState.Terminated; last != nil {
			parts = append(parts, fmt.Sprintf("%s last: %s (exit %d)", c.Name, last.Reason, last.ExitCode))
		}
		if term := c.State.Terminated; term != nil {
			parts = append(parts, fmt.Sprintf("%s terminated: %s (exit %d)", c.Name, term.Reason, term.ExitCode))
		}
	}
	return strings.Join(parts, ", ")
}