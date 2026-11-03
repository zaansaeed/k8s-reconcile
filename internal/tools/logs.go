package tools

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
)


func (t *Tools) GetLogs(ctx context.Context, pod string) (string, error) {
	logBytes, err := t.client.CoreV1().Pods(t.namespace).GetLogs(pod, &corev1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("listing logs in %s for pod %s: %w", t.namespace,pod, err)
	}

}