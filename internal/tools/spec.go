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
			Description: "List every pod in the namespace with its phase, ready count, restart count, and the reason any container is unhealthy. Start here.",
			Properties:  map[string]any{},
		},
		{
			Name:        "get_events",
			Description: "List the namespace's recent events, oldest first, with repeat counts. Events explain image pull and scheduling failures that pod status alone does not.",
			Properties:  map[string]any{},
		},
		{
			Name:        "get_deployments",
			Description: "List every deployment in the namespace with its rollout conditions and the pod template it asks for: each container's image, command, args, ports, env vars and where their values come from, volume mounts, probes and resource requests. This is the desired state; compare it with what pods and events report. Secret values are never shown.",
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
