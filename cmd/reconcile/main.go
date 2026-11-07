package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/zaansaeed/k8s-reconcile/internal/agent"
	"github.com/zaansaeed/k8s-reconcile/internal/llm"
	"github.com/zaansaeed/k8s-reconcile/internal/telemetry"
	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("finding home dir: %v", err)
	}

	//potential flags
	kubeconfig := flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig")
	namespace := flag.String("namespace", "default", "namespace the tools operate in")
	task := flag.String("task", "", "what to ask the agent; defaults to a diagnose request for the namespace") //first user message (after cached tools+system)
	maxSteps := flag.Int("max-steps", 12, "model calls before the agent gives up")
	timeout := flag.Duration("timeout", 5*time.Minute, "deadline for the whole run")
	verbose := flag.Bool("verbose", false, "print each step as the agent takes it")
	dump := flag.Bool("dump", false, "print raw tool output and exit instead of running the agent")
	pod := flag.String("pod", "", "with -dump, pod to read logs from; skipped when empty")
	previous := flag.Bool("previous", false, "with -dump, read the previous container instance's log")
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
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	kube := tools.New(clientset, *namespace)

	if *dump {
		if err := dumpTools(ctx, kube, *pod, *previous); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *task == "" {
		*task = fmt.Sprintf("Namespace %s has a broken workload. Diagnose the root cause.", *namespace)
	}

	client := llm.New(llm.DefaultConfig(agent.SystemPrompt), tools.Specs())
	run := &telemetry.Run{Model: client.Model()}
	diagnoser := agent.New(client, kube, run, *maxSteps)
	if *verbose {
		diagnoser.OnStep = printStep
	}

	result, err := diagnoser.Run(ctx, *task)
	if err != nil {
		log.Fatalf("agent run: %v", err)
	}

	if result.Exhausted {
		fmt.Printf("no answer: gave up after %d steps\n", result.Steps)
	} else {
		fmt.Printf("%s\n", result.Answer)
	}

	fmt.Printf("\nsteps %d  in %d (uncached 1x)  out %d  cache_write %d (1.25x)  cache_read %d (0.05x)\n",
		run.Steps,
		run.Usage.InputTokens,
		run.Usage.OutputTokens,
		run.Usage.CacheWriteTokens,
		run.Usage.CacheReadTokens,
	)
	if cost, known := run.CostUSD(); known {
		fmt.Printf("cost $%.4f (%s)\n", cost, run.Model)
	}
}

func printStep(info agent.StepInfo) {
	fmt.Printf("step %d  %s  in %d (uncached 1x)  out %d  cache_write %d (1.25x)  cache_read %d (0.05x)\n",
		info.N,
		info.Elapsed.Round(time.Millisecond),
		info.Usage.InputTokens,
		info.Usage.OutputTokens,
		info.Usage.CacheWriteTokens,
		info.Usage.CacheReadTokens,
	)

	if text := strings.TrimSpace(info.Text); text != "" {
		fmt.Printf("  %s\n", firstLine(text, 160))
	}

	for _, call := range info.Calls {
		fmt.Printf("  model use -> %s %s\n", call.Name, call.Input)
		output := info.Results[call.ID]
		fmt.Printf("     %s  (%d bytes)\n", firstLine(output, 120), len(output))
	}

	fmt.Println()
}

func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}

func dumpTools(ctx context.Context, kube *tools.Tools, pod string, previous bool) error {
	pods, err := kube.GetPods(ctx)
	if err != nil {
		return fmt.Errorf("get pods: %w", err)
	}
	fmt.Print(pods)

	events, err := kube.GetEvents(ctx)
	if err != nil {
		return fmt.Errorf("get events: %w", err)
	}
	fmt.Print(events)

	if pod == "" {
		return nil
	}
	logs, err := kube.GetLogs(ctx, pod, previous)
	if err != nil {
		return fmt.Errorf("get logs: %w", err)
	}
	fmt.Print(logs)
	return nil
}
