package tools

type Spec struct {
	Name        string
	Description string
	Properties  map[string]any //parameters
	Required    []string
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
