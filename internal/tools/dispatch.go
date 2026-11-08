package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Dispatch runs the tool the model named, with the raw JSON arguments it sent.
func (t *Tools) Dispatch(ctx context.Context, name string, input json.RawMessage) (string, error) {
	switch name {
	case "get_pods":
		return t.GetPods(ctx)
	case "get_events":
		return t.GetEvents(ctx)
	case "get_deployments":
		return t.GetDeployments(ctx)
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
	case "apply_patch":
		if !t.allowWrites {
			return "", fmt.Errorf("tool %q called but writes are not enabled", name)
		}
		var args struct {
			Kind  string `json:"kind"`
			Name  string `json:"name"`
			Patch string `json:"patch"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return fmt.Sprintf("error: apply_patch arguments were not valid JSON: %v\n", err), nil
		}
		if args.Name == "" {
			return "error: apply_patch requires an object name\n", nil
		}
		return t.ApplyPatch(ctx, args.Kind, args.Name, args.Patch)
	case "wait_for_rollout":
		if !t.allowWrites {
			return "", fmt.Errorf("tool %q called but writes are not enabled", name)
		}
		var args struct {
			Name           string `json:"name"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return fmt.Sprintf("error: wait_for_rollout arguments were not valid JSON: %v\n", err), nil
		}
		if args.Name == "" {
			return "error: wait_for_rollout requires a deployment name\n", nil
		}
		return t.WaitForRollout(ctx, args.Name, time.Duration(args.TimeoutSeconds)*time.Second)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}
