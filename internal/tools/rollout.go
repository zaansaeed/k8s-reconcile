package tools

import (
	"context"
	"fmt"
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

// rolloutPoll is how often the deployment is re-read while waiting, and
// rolloutSettle is how long the new pods must stay up before a rollout counts
// as complete. Both are variables so a quick check can shorten them.
var (
	rolloutPoll   = 2 * time.Second
	rolloutSettle = 10 * time.Second
)

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
func (t *Toolbox) WaitForRollout(ctx context.Context, name string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = defaultRolloutWait
	}
	timeout = min(timeout, maxRolloutWait)

	deadline := time.Now().Add(timeout)
	var stableSince time.Time
	for {
		d, err := t.client.AppsV1().Deployments(t.namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("no deployment named %s in namespace %s\n", name, t.namespace), nil
		}
		if err != nil {
			return "", fmt.Errorf("reading deployment %s: %w", name, err)
		}

		state, detail := rolloutStatus(d)
		if state == rolloutComplete { // deployed is not the same as staying up, so check the pods too
			problem, err := t.unstablePod(ctx, d)
			if err != nil {
				return "", err
			}
			if problem != "" {
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s deployed but not stable: %s.", name, problem))
			}
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= rolloutSettle {
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s complete: %s, and stayed up without restarting for %s.", name, detail, rolloutSettle))
			}
		} else {
			stableSince = time.Time{}
			if state == rolloutFailed {
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s failed: %s.", name, detail))
			}
			if !time.Now().Before(deadline) {
				return t.rolloutReport(ctx, fmt.Sprintf("rollout of deployment/%s not finished after %s: %s.", name, timeout, detail))
			}
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

	desired := desiredReplicas(d)
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

// unstablePod looks for a pod of the deployment that has restarted, is waiting
// or has exited. Without a readiness probe, a crash-looping container counts as
// available in the moments it runs, so rollout status alone can't catch it.
// It returns "" when every pod looks stable.
func (t *Toolbox) unstablePod(ctx context.Context, d *appsv1.Deployment) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("reading selector of deployment %s: %w", d.Name, err)
	}
	pods, err := t.client.CoreV1().Pods(t.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", fmt.Errorf("listing pods of deployment %s: %w", d.Name, err)
	}

	for _, p := range pods.Items {
		if p.DeletionTimestamp != nil || p.Status.Phase == corev1.PodSucceeded { // old pods on their way out
			continue
		}
		for _, c := range p.Status.ContainerStatuses {
			switch {
			case c.State.Waiting != nil:
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

// rolloutReport follows the verdict's headline with every pod in the
// namespace, since a fix to one deployment can leave another still broken.
func (t *Toolbox) rolloutReport(ctx context.Context, headline string) (string, error) {
	pods, err := t.GetPods(ctx)
	if err != nil {
		return "", err
	}
	return headline + "\npods now:\n" + pods, nil
}
