package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
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
	//potential flags
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig; defaults to $KUBECONFIG, then ~/.kube/config")
	namespace := flag.String("namespace", "default", "namespace the tools operate in")
	task := flag.String("task", "", "what to ask the agent; defaults to a diagnose request for the namespace") //first user message (after cached tools+system)
	model := flag.String("model", "opus", "model to run: opus, sonnet, haiku, or a full model ID")
	maxSteps := flag.Int("max-steps", 0, "model calls before the agent gives up; defaults to 12, or 25 with -allow-writes")
	timeout := flag.Duration("timeout", 0, "deadline for the whole run; defaults to 5m, or 15m with -allow-writes")
	verbose := flag.Bool("verbose", false, "print each step as the agent takes it")
	dump := flag.Bool("dump", false, "print raw tool output and exit instead of running the agent")
	pod := flag.String("pod", "", "with -dump, pod to read logs from; skipped when empty")
	previous := flag.Bool("previous", false, "with -dump, read the previous container instance's log")
	allowWrites := flag.Bool("allow-writes", false, "let the agent patch deployments and wait for rollouts; uses the repair prompt")
	flag.Parse()

	prompt, instruction, steps, deadline := agent.DiagnosePrompt, "Diagnose the root cause.", 12, 5*time.Minute
	if *allowWrites { // repair runs wait on rollouts, so they get more steps and time
		prompt, instruction, steps, deadline = agent.RepairPrompt, "Diagnose and repair it.", 25, 15*time.Minute
	}
	if *maxSteps == 0 {
		*maxSteps = steps
	}
	if *timeout == 0 {
		*timeout = deadline
	}
	if *task == "" {
		*task = fmt.Sprintf("Namespace %s has a broken workload. %s", *namespace, instruction)
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules() // the same lookup kubectl uses
	rules.ExplicitPath = *kubeconfig
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		log.Fatalf("loading kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("creating clientset: %v", err)
	}

	// One deadline shared by every request in this run.
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	toolbox := tools.New(clientset, *namespace)

	if *dump {
		if err := dumpTools(ctx, toolbox, *pod, *previous); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *allowWrites {
		toolbox.EnableWrites()
	}

	client := llm.New(llm.DefaultConfig(prompt, *model), toolbox.Specs())
	run := &telemetry.Run{Model: client.Model()}
	ag := agent.New(client, toolbox, run, *maxSteps) //init agent
	if *verbose {
		ag.OnStep = func(info agent.StepInfo) { printStep(run, info) }
	}

	result, err := ag.Run(ctx, *task)
	if err != nil {
		log.Fatalf("agent run: %v", err)
	}

	if result.Exhausted {
		fmt.Printf("no answer: gave up after %d steps\n", result.Steps)
	} else {
		fmt.Printf("%s\n", result.Answer)
	}
	fmt.Println("\n------------TOTAL METRICS------------")
	fmt.Printf("\nsteps %d  %s\n", run.Steps, run.UsageLine(run.Usage))
	if cost, known := run.CostUSD(); known {
		fmt.Printf("cost $%.4f (%s)\n", cost, run.Model)
	} else {
		fmt.Printf("cost unknown: no price for %s in telemetry/cost.go\n", run.Model)
	}

	if *allowWrites {
		applied := toolbox.Applied()
		fmt.Printf("\n------------PATCHES APPLIED (%d)------------\n", len(applied))
		for i, p := range applied {
			fmt.Printf("%d. %s/%s  %s\n", i+1, strings.ToLower(p.Kind), p.Name, p.Patch)
		}
	}
}

func printStep(run *telemetry.Run, info agent.StepInfo) {
	fmt.Printf("step %d  %s  %s\n", info.N, info.Elapsed.Round(time.Millisecond), run.UsageLine(info.Usage))

	if text := strings.TrimSpace(info.Text); text != "" && len(info.Calls) != 0 {
		fmt.Printf("  %s\n", firstLine(text, 160))
	}

	for _, call := range info.Calls {
		fmt.Printf("  model use -> %s %s\n", call.Name, call.Input)
		fmt.Printf("     %s  (%d bytes)\n", firstLine(call.Output, 120), len(call.Output))
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

func dumpTools(ctx context.Context, toolbox *tools.Toolbox, pod string, previous bool) error {
	args, _ := json.Marshal(map[string]any{"pod": pod, "previous": previous}) // tools that take no arguments ignore these
	for _, spec := range toolbox.Specs() {
		if spec.Name == "get_logs" && pod == "" {
			continue
		}
		out, err := toolbox.Dispatch(ctx, spec.Name, args)
		if err != nil {
			return fmt.Errorf("%s: %w", spec.Name, err)
		}
		fmt.Printf("\n ------------Tool------------ %s\n", spec.Name)
		fmt.Print(out)
	}
	return nil
}
