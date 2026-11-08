package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// maxPatchBytes caps a patch's size. Real fixes are a few hundred bytes; a
// huge patch is a sign the model is rewriting the whole object.
const maxPatchBytes = 8 * 1024

// ApplyPatch checks a patch locally, has the API server validate it in a dry
// run, then applies it with a strategic merge patch, which merges list items by
// key so a patch can change one container without restating the others. Every
// rejection is returned as text, not an error, so the model can read why and
// try again instead of the run ending.
func (t *Toolbox) ApplyPatch(ctx context.Context, kind string, name string, patch string) (string, error) {
	if len(patch) > maxPatchBytes {
		return fmt.Sprintf("patch rejected: %d bytes, the limit is %d\n", len(patch), maxPatchBytes), nil
	}
	if reason := checkPatch(kind, patch); reason != "" {
		return "patch rejected: " + reason + "\n", nil
	}

	data := []byte(patch)
	deployments := t.client.AppsV1().Deployments(t.namespace)
	dryRun := metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}}
	if _, err := deployments.Patch(ctx, name, types.StrategicMergePatchType, data, dryRun); err != nil {
		return fmt.Sprintf("patch rejected by the API server (dry run, nothing changed): %v\n", err), nil
	}
	patched, err := deployments.Patch(ctx, name, types.StrategicMergePatchType, data, metav1.PatchOptions{}) // actually apply the patch
	if err != nil {
		return fmt.Sprintf("patch failed after passing the dry run: %v\n", err), nil
	}
	t.applied = append(t.applied, AppliedPatch{Kind: kind, Name: name, Patch: patch})

	// Show the object as the server now has it, so the model sees what actually
	// changed, including surprises like a duplicated volume mount.
	var b strings.Builder
	fmt.Fprintf(&b, "patched deployment/%s. It now reads:\n", name)
	writeDeployment(&b, *patched)
	return b.String(), nil
}

// checkPatch enforces the one rule without calling the API, so a disallowed
// patch never reaches the cluster: only a Deployment's spec.template may
// change. Replicas would let an agent "fix" a Deployment by scaling it to zero,
// and selectors are immutable. Keys are checked in sorted order so the same
// patch always gets the same message. It returns "" when allowed.
func checkPatch(kind, patch string) string {
	if kind != "Deployment" {
		return fmt.Sprintf("kind %q cannot be patched; only Deployment can", kind)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(patch), &top); err != nil {
		return fmt.Sprintf("patch is not a JSON object: %v", err)
	}
	if len(top) == 0 {
		return "patch is empty"
	}

	for _, field := range sortedKeys(top) {
		if field != "spec" {
			return fmt.Sprintf("a Deployment patch may only change spec.template, not %q", field)
		}
		var spec map[string]json.RawMessage
		if err := json.Unmarshal(top[field], &spec); err != nil {
			return `"spec" must be a JSON object`
		}
		for _, sub := range sortedKeys(spec) {
			if sub != "template" {
				return fmt.Sprintf("a Deployment patch may only change spec.template, not spec.%s", sub)
			}
		}
	}
	return ""
}

// sortedKeys returns a map's keys in sorted order, since Go randomizes map
// iteration.
func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
