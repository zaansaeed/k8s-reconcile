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

type gradeRecord struct {
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
	gradesPath := flag.String("out", "results/grades.jsonl", "where grades are appended; runs already graded here are skipped")
	judgeModel := flag.String("model", "opus", "judge model")
	onlyScenarios := flag.String("only", "", "comma-separated scenarios to grade; all when empty")
	showInput := flag.Bool("show", false, "print the judge's input for the first matching run and exit, without calling the API")
	flag.Parse()

	alreadyGraded := map[time.Time]bool{}
	for _, existing := range readLines[gradeRecord](*gradesPath) {
		if existing.Error == "" {
			alreadyGraded[existing.Run] = true
		}
	}
	var gradesFile *os.File
	if !*showInput {
		var err error
		if gradesFile, err = os.OpenFile(*gradesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err != nil {
			log.Fatal(err)
		}
		defer gradesFile.Close()
	}

	judgeClient := llm.New(llm.DefaultConfig(rubric, *judgeModel), []tools.Spec{gradeTool})
	judgeTotals := telemetry.Run{Model: judgeClient.Model()}
	for _, run := range readLines[agent.Record](*runsPath) {
		if alreadyGraded[run.Time] || (*onlyScenarios != "" && !slices.Contains(strings.Split(*onlyScenarios, ","), run.Scenario)) {
			continue
		}
		if *showInput {
			prompt, err := judgeInput(run)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(prompt)
			return
		}
		record := gradeRun(judgeClient, run)
		judgeTotals.Record(record.Usage)
		line, _ := json.Marshal(record)
		gradesFile.Write(append(line, '\n'))
		fmt.Printf("%s %s %s: %s%s\n", run.Scenario, run.Model, run.Mode, overallVerdict(record.Grade), record.Error)
	}
	totalCost, _ := judgeTotals.CostUSD()
	fmt.Printf("\njudge cost $%.4f  %s\n", totalCost, judgeTotals.UsageLine(judgeTotals.Usage))
}

func gradeRun(judgeClient *llm.Client, run agent.Record) gradeRecord {
	record := gradeRecord{Run: run.Time, Scenario: run.Scenario, Model: run.Model, Mode: run.Mode, Judge: judgeClient.Model()}
	prompt, err := judgeInput(run)
	if err != nil {
		record.Error = err.Error()
		return record
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	reply, err := judgeClient.Send(ctx, []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))})
	if err != nil {
		record.Error = err.Error()
		return record
	}
	record.Usage = reply.Usage
	callCost := telemetry.Run{Model: judgeClient.Model()}
	callCost.Record(reply.Usage)
	record.CostUSD, _ = callCost.CostUSD()
	if len(reply.Calls) != 1 || reply.Calls[0].Name != gradeTool.Name {
		record.Error = "judge did not call record_grade: " + reply.Text
		return record
	}
	record.Grade = reply.Calls[0].Input
	return record
}

func judgeInput(run agent.Record) (string, error) {
	manifest, manifestErr := os.ReadFile(filepath.Join("scenarios", run.Scenario+".yaml"))
	truth, truthErr := os.ReadFile(filepath.Join("scenarios", "truths", run.Scenario+".yaml"))
	if err := errors.Join(manifestErr, truthErr); err != nil {
		return "", err
	}
	return fmt.Sprintf("<manifest>\n%s</manifest>\n\n<truth>\n%s</truth>\n\n<transcript>\n%s</transcript>", manifest, truth, transcript(run)), nil
}

func transcript(run agent.Record) string {
	var text strings.Builder
	fmt.Fprintf(&text, "mode: %s\n", run.Mode)
	for _, step := range run.Steps {
		fmt.Fprintf(&text, "\n--- step %d\n", step.N)
		if narration := strings.TrimSpace(step.Text); narration != "" && len(step.Calls) > 0 {
			fmt.Fprintf(&text, "agent: %s\n", narration)
		}
		for _, call := range step.Calls {
			fmt.Fprintf(&text, "call %s %s\nresult:\n%s\n", call.Name, call.Input, call.Output)
		}
	}
	switch {
	case run.Error != "":
		fmt.Fprintf(&text, "\nrun ended with an error: %s\n", run.Error)
	case run.Exhausted:
		text.WriteString("\nrun hit its step limit without answering\n")
	default:
		fmt.Fprintf(&text, "\n--- final answer\n%s\n", run.Answer)
	}
	if run.Mode == "repair" {
		fmt.Fprintf(&text, "\n--- patches that reached the cluster: %d\n", len(run.Patches))
		for _, patch := range run.Patches {
			fmt.Fprintf(&text, "%s/%s %s\n", strings.ToLower(patch.Kind), patch.Name, patch.Patch)
		}
		fmt.Fprintf(&text, "\n--- pods right after the run\n%s", run.PodsAfter)
	}
	return text.String()
}

func overallVerdict(gradeJSON json.RawMessage) string {
	var parsed struct{ Overall string }
	json.Unmarshal(gradeJSON, &parsed)
	return parsed.Overall
}

func readLines[T any](path string) []T {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var items []T
	scanner := bufio.NewScanner(file)
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
