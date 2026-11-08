package tools

type Spec struct {
	Name        string
	Description string
	Properties  map[string]any //parameters
	Required    []string
}

// WriteSpecs describes the tools that change the cluster. They are kept apart
// from Specs so a diagnosis run is never offered them.
func WriteSpecs() []Spec {
	return []Spec{
		{
			Name: "apply_patch",
			Description: "Change a Deployment's pod template with a strategic merge patch. The patch is checked by the API server in a dry run first; if it is rejected, nothing changes and you get the error. On success you get the deployment as it now reads. " +
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
		{
			Name: "wait_for_rollout",
			Description: "Wait for a deployment's rollout to finish after a patch, then report the outcome and the current state of every pod in the namespace. " +
				"Complete means every pod runs the new template, is available, and has stayed up for a short settling period without restarting. A rollout can be deployed but not stable, for example when the new pods keep crashing; the report says which pod and why. If it is not finished when the timeout ends, the report says what it is still waiting for. " +
				"A fix can reveal a second fault that the first one was hiding, so read the pods in the report before deciding you are done.",
			Properties: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the deployment, exactly as get_deployments reported it.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "How long to wait, from 1 to 120 seconds. Defaults to 60.",
				},
			},
			Required: []string{"name"},
		},
	}
}

// Specs describes the read-only tools to the model, in a fixed order.
func Specs() []Spec {
	return []Spec{
		{
			Name:        "get_pods",
			Description: "List every pod in the namespace with its phase, ready count, restart count (none, 1, several or many), and for any unhealthy container its waiting reason and last exit code.",
			Properties:  map[string]any{},
		},
		{
			Name:        "get_events",
			Description: "List the namespace's 20 most recent events, oldest first. Events that happened more than once are marked (repeated). Events explain image pull, scheduling and probe failures that pod status alone does not. Events expire about an hour after they last occur, so one-time events from an older failure may be missing.",
			Properties:  map[string]any{},
		},
		{
			Name:        "get_deployments",
			Description: "List every deployment in the namespace with its rollout conditions and the pod template it asks for: each container's image, command, args, ports, env vars and where their values come from, volume mounts, probes and resource requests. This is the desired state; compare it with what pods and events report. Secret values are never shown.",
			Properties:  map[string]any{},
		},
		{
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
	}
}
