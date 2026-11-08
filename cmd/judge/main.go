package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/zaansaeed/k8s-reconcile/internal/agent"
	"github.com/zaansaeed/k8s-reconcile/internal/llm"
	"github.com/zaansaeed/k8s-reconcile/internal/telemetry"
	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

type run struct {
	Time      time.Time
	Scenario  string
	Model     string
	Mode      string
	Steps     []agent.StepInfo
	Answer    string
	Exhausted bool
	Error     string
	Patches   []tools.AppliedPatch
	PodsAfter string
}

type grade struct {
	Run      time.Time
	Scenario string
	Model    string
	Mode     string
	Judge    string
	Grade    json.RawMessage
	Error    string
	Usage    llm.Usage
	CostUSD  float64
}

func main() {
	runsPath := flag.String("runs", "results/runs.jsonl", "runs to grade")
	outPath := flag.String("out", "results/grades.jsonl", "where grades are appended; runs already graded here are skipped")
	model := flag.String("model", "opus", "judge model")
	only := flag.String("only", "", "comma-separated scenarios to grade; all when empty")
	show := flag.Bool("show", false, "print the judge's input for the first matching run and exit, without calling the API")
	flag.Parse()

	graded := map[time.Time]bool{}
	for _, g := range readLines[grade](*outPath) {
		if g.Error == "" {
			graded[g.Run] = true
		}
	}
	var out *os.File
	if !*show {
		var err error
		if out, err = os.OpenFile(*outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err != nil {
			log.Fatal(err)
		}
		defer out.Close()
	}

	client := llm.New(llm.DefaultConfig(rubric, *model), []tools.Spec{gradeTool})
	total := telemetry.Run{Model: client.Model()}
	for _, r := range readLines[run](*runsPath) {
		if graded[r.Time] || (*only != "" && !slices.Contains(strings.Split(*only, ","), r.Scenario)) {
			continue
		}
		if *show {
			prompt, err := input(r)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(prompt)
			return
		}
		g := judge(client, r)
		total.Record(g.Usage)
		line, _ := json.Marshal(g)
		out.Write(append(line, '\n'))
		fmt.Printf("%s %s %s: %s%s\n", r.Scenario, r.Model, r.Mode, overall(g.Grade), g.Error)
	}
	cost, _ := total.CostUSD()
	fmt.Printf("\njudge cost $%.4f  %s\n", cost, total.UsageLine(total.Usage))
}

func judge(client *llm.Client, r run) grade {
	g := grade{Run: r.Time, Scenario: r.Scenario, Model: r.Model, Mode: r.Mode, Judge: client.Model()}
	prompt, err := input(r)
	if err != nil {
		g.Error = err.Error()
		return g
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	reply, err := client.Send(ctx, []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))})
	if err != nil {
		g.Error = err.Error()
		return g
	}
	g.Usage = reply.Usage
	cost := telemetry.Run{Model: client.Model()}
	cost.Record(reply.Usage)
	g.CostUSD, _ = cost.CostUSD()
	if len(reply.Calls) != 1 || reply.Calls[0].Name != gradeTool.Name {
		g.Error = "judge did not call record_grade: " + reply.Text
		return g
	}
	g.Grade = reply.Calls[0].Input
	return g
}

func input(r run) (string, error) {
	manifest, err1 := os.ReadFile(filepath.Join("scenarios", r.Scenario+".yaml"))
	truth, err2 := os.ReadFile(filepath.Join("scenarios", "truths", r.Scenario+".yaml"))
	if err := errors.Join(err1, err2); err != nil {
		return "", err
	}
	return fmt.Sprintf("<manifest>\n%s</manifest>\n\n<truth>\n%s</truth>\n\n<transcript>\n%s</transcript>", manifest, truth, transcript(r)), nil
}

func transcript(r run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mode: %s\n", r.Mode)
	for _, s := range r.Steps {
		fmt.Fprintf(&b, "\n--- step %d\n", s.N)
		if t := strings.TrimSpace(s.Text); t != "" && len(s.Calls) > 0 {
			fmt.Fprintf(&b, "agent: %s\n", t)
		}
		for _, c := range s.Calls {
			fmt.Fprintf(&b, "call %s %s\nresult:\n%s\n", c.Name, c.Input, c.Output)
		}
	}
	switch {
	case r.Error != "":
		fmt.Fprintf(&b, "\nrun ended with an error: %s\n", r.Error)
	case r.Exhausted:
		b.WriteString("\nrun hit its step limit without answering\n")
	default:
		fmt.Fprintf(&b, "\n--- final answer\n%s\n", r.Answer)
	}
	if r.Mode == "repair" {
		fmt.Fprintf(&b, "\n--- patches that reached the cluster: %d\n", len(r.Patches))
		for _, p := range r.Patches {
			fmt.Fprintf(&b, "%s/%s %s\n", strings.ToLower(p.Kind), p.Name, p.Patch)
		}
		fmt.Fprintf(&b, "\n--- pods right after the run\n%s", r.PodsAfter)
	}
	return b.String()
}

func overall(raw json.RawMessage) string {
	var g struct{ Overall string }
	json.Unmarshal(raw, &g)
	return g.Overall
}

func readLines[T any](path string) []T {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var items []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 16<<20)
	for scanner.Scan() {
		var item T
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		items = append(items, item)
	}
	return items
}
