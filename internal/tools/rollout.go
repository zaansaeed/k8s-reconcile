package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultRolloutWait is used when the model gives no timeout. maxRolloutWait
// stops one call from eating most of the run's time budget.
const (
	defaultRolloutWait = 60 * time.Second
	maxRolloutWait     = 120 * time.Second
)

// rolloutPoll is how often the deployment is re-read while waiting. It is a
// variable so a quick check can shorten it.
var rolloutPoll = 2 * time.Second

// rolloutState is the verdict on a rollout at one moment.
type rolloutState int

const (
	rolloutInProgress rolloutState = iota
	rolloutComplete
	rolloutFailed
)

// WaitForRollout polls a deployment until its rollout completes, fails, or the
// timeout ends, then reports the outcome. Its feedback is how the agent learns
// whether a fix worked, or exposed a fault the first one was hiding.
func (t *Tools) WaitForRollout(ctx context.Context, name string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = defaultRolloutWait
	}
	if timeout > maxRolloutWait {
		timeout = maxRolloutWait
	}

	deadline := time.Now().Add(timeout)
	for {
		d, err := t.client.AppsV1().Deployments(t.namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("no deployment named %s in namespace %s\n", name, t.namespace), nil
		}
		if err != nil {
			return "", fmt.Errorf("reading deployment %s: %w", name, err)
		}

		state, detail := rolloutStatus(d)
		if state != rolloutInProgress || !time.Now().Before(deadline) {
			return t.rolloutReport(ctx, d, state, detail, timeout)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(rolloutPoll):
		}
	}
}

// rolloutStatus applies the same checks as kubectl rollout status, in order.
// The generation check comes first because right after a patch the status
// still describes the old version and would wrongly read as complete.
func rolloutStatus(d *appsv1.Deployment) (rolloutState, string) {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse && c.Reason == "ProgressDeadlineExceeded" {
			return rolloutFailed, "the deployment controller reports ProgressDeadlineExceeded"
		}
	}

	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	s := d.Status
	switch {
	case s.ObservedGeneration < d.Generation:
		return rolloutInProgress, "the controller has not processed the latest change yet"
	case s.UpdatedReplicas < desired:
		return rolloutInProgress, fmt.Sprintf("%d of %d pods run the new template", s.UpdatedReplicas, desired)
	case s.Replicas > s.UpdatedReplicas:
		return rolloutInProgress, fmt.Sprintf("%d pods from the old template are still running", s.Replicas-s.UpdatedReplicas)
	case s.AvailableReplicas < desired:
		return rolloutInProgress, fmt.Sprintf("%d of %d new pods are available", s.AvailableReplicas, desired)
	default:
		return rolloutComplete, fmt.Sprintf("all %d pods run the new template and are available", desired)
	}
}

// rolloutReport states the verdict and why, then lists every pod in the
// namespace, since a fix to one deployment can leave another still broken.
func (t *Tools) rolloutReport(ctx context.Context, d *appsv1.Deployment, state rolloutState, detail string, waited time.Duration) (string, error) {
	var b strings.Builder
	switch state {
	case rolloutComplete:
		fmt.Fprintf(&b, "rollout of deployment/%s complete: %s.\n", d.Name, detail)
	case rolloutFailed:
		fmt.Fprintf(&b, "rollout of deployment/%s failed: %s.\n", d.Name, detail)
	default:
		fmt.Fprintf(&b, "rollout of deployment/%s not finished after %s: %s.\n", d.Name, waited, detail)
	}

	pods, err := t.GetPods(ctx)
	if err != nil {
		return "", err
	}
	b.WriteString("pods now:\n")
	b.WriteString(pods)
	return b.String(), nil
}
