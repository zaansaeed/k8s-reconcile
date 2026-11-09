package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/zaansaeed/k8s-reconcile/internal/agent"
)

func recreate(ctx context.Context, name string) (string, error) {
	raw, err := os.ReadFile(filepath.Join("scenarios", "truths", name+".yaml"))
	if err != nil {
		return "", err
	}
	var truth struct {
		Namespace string          `json:"namespace"`
		Settle    metav1.Duration `json:"settle"`
	}
	if err := yaml.Unmarshal(raw, &truth); err != nil {
		return "", fmt.Errorf("reading truth for %s: %w", name, err)
	}
	// The scenarios' shell loops ignore SIGTERM, so a normal delete waits out
	// each pod's 30s grace period. These pods are being thrown away anyway.
	exec.CommandContext(ctx, "kubectl", "delete", "pods", "--all", "-n", truth.Namespace,
		"--grace-period=0", "--force", "--ignore-not-found").Run()
	if out, err := exec.CommandContext(ctx, "kubectl", "delete", "namespace", truth.Namespace,
		"--ignore-not-found", "--wait=true", "--timeout=3m").CombinedOutput(); err != nil {
		return "", fmt.Errorf("deleting namespace %s: %v: %s", truth.Namespace, err, out)
	}
	if out, err := exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join("scenarios", name+".yaml")).CombinedOutput(); err != nil {
		return "", fmt.Errorf("applying %s: %v: %s", name, err, out)
	}
	settle := truth.Settle.Duration
	if settle == 0 {
		settle = 10 * time.Second
	}

	fmt.Printf("\n NEW SCENARIO: %s \n", truth.Namespace)
	fmt.Printf("recreated %s, waiting %s for it to settle\n\n", truth.Namespace, settle)
	time.Sleep(settle)
	return truth.Namespace, nil
}

func save(rec agent.Record, path string) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
