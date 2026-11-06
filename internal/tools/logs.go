package tools

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const maxLogLines = 50

// get logs for a specific pod. If previous = true, returns previous container instance within same pod
// max of 50 log lines
func (t *Tools) GetLogs(ctx context.Context, pod string, previous bool) (string, error) {
	tail := int64(maxLogLines)
	opts := &corev1.PodLogOptions{
		TailLines: &tail,
		Previous:  previous,
	}

	stream, err := t.client.CoreV1().Pods(t.namespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		if apierrors.IsBadRequest(err) {
			return fmt.Sprintf("no logs available for pod %s: %s\n", pod, err.Error()), nil
		}
		return "", fmt.Errorf("retrieving logs for pod %s in %s (previous=%t): %w", pod, t.namespace, previous, err)
	}
	defer stream.Close()

	logBytes, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("reading log stream for pod %s in %s: %w", pod, t.namespace, err)
	}

	if len(logBytes) == 0 {
		return fmt.Sprintf("pod %s in namespace %s produced no log output\n", pod, t.namespace), nil
	}
	return string(logBytes), nil
}
