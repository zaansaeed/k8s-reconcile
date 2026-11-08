package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/zaansaeed/k8s-reconcile/internal/agent"
	"github.com/zaansaeed/k8s-reconcile/internal/llm"
	"github.com/zaansaeed/k8s-reconcile/internal/telemetry"
	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig; defaults to $KUBECONFIG, then ~/.kube/config")
	namespace := flag.String("namespace", "", "namespace to check as it is now")
	scenario := flag.String("scenario", "", "scenario to recreate and run, e.g. crashloop; the run is appended to results/runs.jsonl")
	model := flag.String("model", "opus", "model to run: opus, sonnet, haiku, or a full model ID")
	verbose := flag.Bool("verbose", false, "print each step as the agent takes it")
	dump := flag.Bool("dump", false, "print raw tool output and exit instead of running the agent")
	pod := flag.String("pod", "", "with -dump, pod to read logs from; skipped when empty")
	previous := flag.Bool("previous", false, "with -dump, read the previous container instance's log")
	allowWrites := flag.Bool("allow-writes", false, "let the agent patch deployments; uses the repair prompt")
	flag.Parse()

	clientset, err := tools.Connect(*kubeconfig)
	if err != nil {
		log.Fatal(err)
	}
	if *scenario != "" {
		if *namespace, err = recreate(context.Background(), *scenario); err != nil {
			log.Fatal(err)
		}
	}
	if *namespace == "" {
		log.Fatal("set -namespace or -scenario")
	}
	toolbox := tools.New(clientset, *namespace)

	mode, prompt, ask, maxSteps, timeout := "diagnose", agent.DiagnosePrompt, "Diagnose the root cause.", 12, 5*time.Minute
	if *allowWrites {
		// Repair runs wait on rollouts, so they get more steps and more time.
		mode, prompt, ask, maxSteps, timeout = "repair", agent.RepairPrompt, "Diagnose and repair it.", 25, 15*time.Minute
		toolbox.EnableWrites()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if *dump {
		if err := dumpTools(ctx, toolbox, *pod, *previous); err != nil {
			log.Fatal(err)
		}
		return
	}

	client := llm.New(llm.DefaultConfig(prompt, *model), toolbox.Specs())
	run := &telemetry.Run{Model: client.Model()}
	rec := record{Time: time.Now(), Scenario: *scenario, Model: run.Model, Mode: mode}
	ag := agent.New(client, toolbox, run, maxSteps)
	ag.OnStep = func(info agent.StepInfo) {
		rec.Steps = append(rec.Steps, info)
		if *verbose {
			printStep(run, info)
		}
	}

	result, err := ag.Run(ctx, fmt.Sprintf("Namespace %s has a broken workload. %s", *namespace, ask))
	rec.Seconds = time.Since(rec.Time).Seconds()
	switch {
	case err != nil:
		rec.Error = err.Error()
		fmt.Printf("agent run: %v\n", err)
	case result.Exhausted:
		rec.Exhausted = true
		fmt.Printf("no answer: gave up after %d steps\n", result.Steps)
	default:
		rec.Answer = result.Answer
		fmt.Printf("%s\n", result.Answer)
	}

	fmt.Println("\n------------TOTAL METRICS------------")
	fmt.Printf("\nsteps %d  %s\n", run.Steps, run.UsageLine(run.Usage))
	rec.Usage = run.Usage
	if cost, known := run.CostUSD(); known {
		rec.CostUSD = cost
		fmt.Printf("cost $%.4f (%s)\n", cost, run.Model)
	} else {
		fmt.Printf("cost unknown: no price for %s in telemetry/cost.go\n", run.Model)
	}

	if *allowWrites {
		rec.Patches = toolbox.Applied()
		fmt.Printf("\n------------PATCHES APPLIED (%d)------------\n", len(rec.Patches))
		for i, p := range rec.Patches {
			fmt.Printf("%d. %s/%s  %s\n", i+1, strings.ToLower(p.Kind), p.Name, p.Patch)
		}
	}

	if *scenario != "" {
		rec.PodsAfter, _ = toolbox.GetPods(context.Background())
		if err := save(rec); err != nil {
			log.Fatal(err)
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
