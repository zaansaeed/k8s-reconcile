// Package agent runs the diagnose loop: ask the model, run the tools it asks
// for, feed the output back, repeat until it answers or the step cap is hit.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/zaansaeed/k8s-reconcile/internal/llm"
	"github.com/zaansaeed/k8s-reconcile/internal/telemetry"
	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

// Agent owns one conversation: the client it talks to, the namespace-scoped
// tools it may run, and the run it records token usage into.
type Agent struct {
	client   *llm.Client
	tools    *tools.Toolbox
	run      *telemetry.Run
	maxSteps int

	OnStep func(StepInfo)
}

type StepInfo struct {
	N       int
	Text    string
	Calls   []llm.ToolCall
	Results map[string]string
	Usage   llm.Usage
	Elapsed time.Duration
}

func (agent *Agent) emit(info StepInfo) {
	if agent.OnStep != nil {
		agent.OnStep(info)
	}
}

// New returns an Agent that gives up after maxSteps model calls.
func New(client *llm.Client, kube *tools.Toolbox, run *telemetry.Run, maxSteps int) *Agent {
	return &Agent{client: client, tools: kube, run: run, maxSteps: maxSteps}
}

// Result is what one run produced.
type Result struct {
	Answer    string
	Steps     int
	Exhausted bool
}

// Run drives the loop until the model stops asking for tools.
func (agent *Agent) Run(ctx context.Context, task string) (*Result, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}

	for step := 1; step <= agent.maxSteps; step++ {
		started := time.Now()
		reply, err := agent.client.Send(ctx, messages)
		elapsed := time.Since(started)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", step, err)
		}
		agent.run.Record(reply.Usage) //record metrics

		switch reply.StopReason {
		case anthropic.StopReasonMaxTokens:
			return nil, fmt.Errorf("step %d: response hit max_tokens, output truncated", step)
		case anthropic.StopReasonRefusal:
			return nil, fmt.Errorf("step %d: model declined the request", step)
		case anthropic.StopReasonModelContextWindowExceeded:
			return nil, fmt.Errorf("step %d: conversation outgrew the context window", step)
		}

		info := StepInfo{
			N:       step,
			Text:    reply.Text,
			Calls:   reply.Calls,
			Usage:   reply.Usage,
			Elapsed: elapsed,
		}

		if len(reply.Calls) == 0 { // no more tool calls. we must be done.
			agent.emit(info)
			return &Result{Answer: reply.Text, Steps: step}, nil
		}

		results := make(map[string]string, len(reply.Calls)) // toolID -> output of tool
		for _, call := range reply.Calls {                   // we have calls to make
			output, err := agent.tools.Dispatch(ctx, call.Name, call.Input)
			if err != nil {
				return nil, fmt.Errorf("step %d: tool %s: %w", step, call.Name, err)
			}
			results[call.ID] = output
		}
		info.Results = results
		agent.emit(info)

		//append the LLMs response as well as our response (tool call results the LLM wanted)
		messages = append(messages, reply.Message.ToParam(), llm.ToolResults(results))
	}

	return &Result{Steps: agent.maxSteps, Exhausted: true}, nil
}
