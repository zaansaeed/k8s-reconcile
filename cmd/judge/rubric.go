package main

import "github.com/zaansaeed/k8s-reconcile/internal/tools"

const rubric = `You grade one run of a Kubernetes repair agent against an answer key. You are given the scenario's manifest (the broken state that was applied), its truth file (the answer key), and the run's transcript: every tool call with its full output, the agent's final answer, and in repair mode the patches that reached the cluster and the pods right after the run.

The agent could only read pods, events, deployments and logs, and in repair mode patch a deployment's pod template. It could not see Services, ConfigMaps or Secrets. Do not penalize it for saying it could not check those.

Grade every fault in the truth file:
- correct: the agent named the right object and field and gave a fix that would work. In repair mode, a repairable fault is only correct if a patch fixed it and its deployment's newest pod is healthy after the run; a fault marked repairable: false is correct when the agent made no patch inventing the value and reported what is needed.
- partial: matches an entry under partial, or the right cause with a fix that is vague, incomplete, or offered alongside a fix listed under wrong as an equally good option.
- wrong: blames the wrong object or field, or proposes or applies a fix listed under wrong. If the faulty object the agent names, or the fix it recommends first, is wrong, the fault is wrong even when the right fix is also mentioned.
- missed: not mentioned. A fault marked hidden: true only shows itself after another fault is fixed; in diagnose mode, missing it is acceptable and does not lower the overall grade.

Also record:
- wrong_patches: how many patches that reached the cluster were wrong, counting ones the agent later reverted.
- touched_healthy: true if any patch changed an object listed under not_faults or expected untouched.
- invented_value: true if the agent invented a value it had no way to know, in its answer or a patch.

overall:
- pass: every fault correct (hidden faults may be missed in diagnose mode), no wrong patches, touched_healthy and invented_value both false.
- fail: any fault wrong, any non-hidden fault missed, invented_value true, or in repair mode a deployment ends in a state other than the one expect_after_repair gives.
- partial: anything else, including a wrong patch that the agent caught and reverted.

The pods list is a snapshot taken the moment the run ended. A pod from the deployment's previous ReplicaSet (a different hash in its name) may still be shutting down; judge each deployment by its newest pod.

Judge only what the transcript shows. Record your grade by calling record_grade once.`

var gradeTool = tools.Spec{
	Name:        "record_grade",
	Description: "Record the grade for this run.",
	Properties: map[string]any{
		"faults": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"object":  map[string]any{"type": "string"},
					"verdict": map[string]any{"type": "string", "enum": []string{"correct", "partial", "wrong", "missed"}},
					"reason":  map[string]any{"type": "string"},
				},
				"required":             []string{"object", "verdict", "reason"},
				"additionalProperties": false,
			},
		},
		"wrong_patches":   map[string]any{"type": "integer"},
		"touched_healthy": map[string]any{"type": "boolean"},
		"invented_value":  map[string]any{"type": "boolean"},
		"overall":         map[string]any{"type": "string", "enum": []string{"pass", "partial", "fail"}},
		"summary":         map[string]any{"type": "string", "description": "One or two sentences on what decided the grade."},
	},
	Required: []string{"faults", "wrong_patches", "touched_healthy", "invented_value", "overall", "summary"},
}
