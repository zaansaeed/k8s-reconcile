package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type Spec struct {
	Name        string
	Description string
	Properties  map[string]any //parameters
	Required    []string
}

func Specs() []Spec {
	return []Spec{
		{
			Name:        "get_pods",
			Description: "List every pod in the namespace with its phase, ready count, restart count, and the reason any container is unhealthy. Start here.",
			Properties:  map[string]any{},
		},
		{
			Name:        "get_events",
			Description: "List the namespace's recent events, oldest first, with repeat counts. Events explain image pull and scheduling failures that pod status alone does not.",
			Properties:  map[string]any{},
		},
		{
			Name:        "get_logs",
			Description: "Return the last 50 log lines from a pod. Set previous=true to read the log of the container's previous, crashed instance, which is where a CrashLoopBackOff cause is visible.",
			Properties: map[string]any{
				"pod": map[string]any{
					"type":        "string",
					"description": "Pod name, exactly as get_pods reported it.",
				},
				"previous": map[string]any{
					"type":        "boolean",
					"description": "Read the previous container instance's log instead of the current one.",
				},
			},
			Required: []string{"pod"},
		},
	}
}

func (t *Tools) Dispatch(ctx context.Context, name string, input json.RawMessage) (string, error) {
	switch name {
	case "get_pods":
		return t.GetPods(ctx)
	case "get_events":
		return t.GetEvents(ctx)
	case "get_logs":
		var args struct {
			Pod      string `json:"pod"`
			Previous bool   `json:"previous"`
		}
		if err := json.Unmarshal(input, &args); err != nil { //unmarshal unpacks the json into our args struct
			return fmt.Sprintf("error: get_logs arguments were not valid JSON: %v\n", err), nil
		}
		if args.Pod == "" { // pod could be empty.
			return "error: get_logs requires a pod name\n", nil
		}
		return t.GetLogs(ctx, args.Pod, args.Previous)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}
