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

type tool struct {
	spec  Spec
	write bool
	run   func(t *Toolbox, ctx context.Context, input json.RawMessage) (string, error)
}

var registry = []tool{
	{
		spec: Spec{
			Name:        "get_pods",
			Description: "List every pod in the namespace with its phase, ready count, restart count (none, 1, several or many), and for any unhealthy container its waiting reason and last exit code.",
			Properties:  map[string]any{},
		},
		run: func(t *Toolbox, ctx context.Context, _ json.RawMessage) (string, error) { return t.GetPods(ctx) },
	},
	{
		spec: Spec{
			Name:        "get_events",
			Description: "List the namespace's 20 most recent events, oldest first. Events that happened more than once are marked (repeated). Events explain image pull, scheduling and probe failures that pod status alone does not. Events expire about an hour after they last occur, so one-time events from an older failure may be missing.",
			Properties:  map[string]any{},
		},
		run: func(t *Toolbox, ctx context.Context, _ json.RawMessage) (string, error) { return t.GetEvents(ctx) },
	},
	{
		spec: Spec{
			Name:        "get_deployments",
			Description: "List every deployment in the namespace with its rollout conditions and the pod template it asks for: each container's image, command, args, ports, env vars and where their values come from, volume mounts, probes and resource requests. This is the desired state; compare it with what pods and events report. \"env: none\" means the container has no env or envFrom entries at all. Secret values are never shown.",
			Properties:  map[string]any{},
		},
		run: func(t *Toolbox, ctx context.Context, _ json.RawMessage) (string, error) { return t.GetDeployments(ctx) },
	},
	{
		spec: Spec{
			Name:        "get_logs",
			Description: "Return the last 50 log lines from a pod's main container. By default this is the most recent attempt, including one that has already exited, which is usually where a crash's cause is printed.",
			Properties: map[string]any{
				"pod": map[string]any{
					"type":        "string",
					"description": "Pod name, exactly as get_pods reported it.",
				},
				"previous": map[string]any{
					"type":        "boolean",
					"description": "Read the attempt before the most recent one. Only needed when the container is running again and the crash you care about was earlier.",
				},
			},
			Required: []string{"pod"},
		},
		run: func(t *Toolbox, ctx context.Context, input json.RawMessage) (string, error) {
			args, problem := decodeArgs[struct {
				Pod      string `json:"pod"`
				Previous bool   `json:"previous"`
			}]("get_logs", input)
			if problem != "" {
				return problem, nil
			}
			if args.Pod == "" {
				return "error: get_logs requires a pod name\n", nil
			}
			return t.GetLogs(ctx, args.Pod, args.Previous)
		},
	},
	{
		spec: Spec{
			Name: "apply_patch",
			Description: "Change a Deployment's pod template with a strategic merge patch. The patch is checked by the API server in a dry run first; if it is rejected, nothing changes and you get the error. On success you get the deployment as it now reads, and then the rollout result: apply_patch waits up to 60 seconds for the new pods. The result is complete once they have stayed up for a short settling period without restarting, deployed but not stable as soon as a new pod is seen failing (the report says which pod and why), or not finished when the wait ends. It is followed by the current state of every pod in the namespace; a fix can reveal a second fault that the first one was hiding, so read it before deciding you are done. " +
				"A patch may only change spec.template. Replica counts, selectors, other kinds and Secrets cannot be changed. " +
				"Lists are merged by a key rather than replaced: containers, env and volumes by name, but volumeMounts by mountPath, so changing a mount's path with a plain patch adds a second mount instead of moving the first.",
			Properties: map[string]any{
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"Deployment"},
					"description": "Kind of the object to change. Only Deployment is supported.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the deployment, exactly as get_deployments reported it.",
				},
				"patch": map[string]any{
					"type":        "string",
					"description": "The strategic merge patch, as a JSON object encoded in a string. Example: {\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"api\",\"image\":\"busybox:1.36\"}]}}}}",
				},
			},
			Required: []string{"kind", "name", "patch"},
		},
		write: true,
		run: func(t *Toolbox, ctx context.Context, input json.RawMessage) (string, error) {
			args, problem := decodeArgs[struct {
				Kind  string `json:"kind"`
				Name  string `json:"name"`
				Patch string `json:"patch"`
			}]("apply_patch", input)
			if problem != "" {
				return problem, nil
			}
			if args.Name == "" {
				return "error: apply_patch requires an object name\n", nil
			}
			return t.ApplyPatch(ctx, args.Kind, args.Name, args.Patch)
		},
	},
}

// Specs describes the tools this Toolbox will run, in a fixed order: the
// read-only tools, plus the write tools once EnableWrites has been called.
// It reads the same switch as Dispatch, so what the model is offered and what
// it is allowed to run can't disagree.
func (t *Toolbox) Specs() []Spec {
	var specs []Spec
	for _, tl := range registry {
		if !tl.write || t.allowWrites {
			specs = append(specs, tl.spec)
		}
	}
	return specs
}

// Dispatch runs the tool the model named, with the raw JSON arguments it sent.
func (t *Toolbox) Dispatch(ctx context.Context, name string, input json.RawMessage) (string, error) {
	for _, tl := range registry {
		if tl.spec.Name != name {
			continue
		}
		if tl.write && !t.allowWrites {
			return "", fmt.Errorf("tool %q called but writes are not enabled", name)
		}
		return tl.run(t, ctx, input)
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func decodeArgs[T any](tool string, input json.RawMessage) (T, string) {
	var args T
	if err := json.Unmarshal(input, &args); err != nil {
		return args, fmt.Sprintf("error: %s arguments were not valid JSON: %v\n", tool, err)
	}
	return args, ""
}
