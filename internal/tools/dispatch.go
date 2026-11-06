package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// Dispatch runs the tool the model named, with the raw JSON arguments it sent.
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
