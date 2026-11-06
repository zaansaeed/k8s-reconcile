package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("finding home dir: %v", err)
	}
	kubeconfig := flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig")
	namespace := flag.String("namespace", "default", "namespace the tools operate in")
	pod := flag.String("pod", "", "pod to read logs from; skipped when empty")
	previous := flag.Bool("previous", false, "read the previous container instance's log")
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatalf("loading kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("creating clientset: %v", err)
	}

	// One deadline shared by every request in this run.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t := tools.New(clientset, *namespace)
	out, err := t.GetPods(ctx)
	if err != nil {
		log.Fatalf("get pods: %v", err)
	}
	fmt.Print(out)

	events, err := t.GetEvents(ctx)
	if err != nil {
		log.Fatalf("get events: %v", err)
	}

	fmt.Print(events)

	if *pod == "" {
		return
	}
	logs, err := t.GetLogs(ctx, *pod, *previous)
	if err != nil {
		log.Fatalf("get logs: %v", err)
	}
	fmt.Print(logs)
}
