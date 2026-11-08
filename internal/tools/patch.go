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

// allowedFields lists, per kind, the top-level fields and the subfields under
// them that a patch may touch. Only spec.template: replicas would let an agent
// "fix" a Deployment by scaling it to zero, and selectors are immutable.
var allowedFields = map[string]map[string][]string{
	"Deployment": {"spec": {"template"}},
}

// ApplyPatch checks a patch locally, has the API server validate it in a dry
// run, then applies it. Every rejection is returned as text, not an error, so
// the model can read why and try again instead of the run ending.
func (t *Tools) ApplyPatch(ctx context.Context, kind string, name string, patch string) (string, error) {
	if len(patch) > maxPatchBytes {
		return fmt.Sprintf("patch rejected: %d bytes, the limit is %d\n", len(patch), maxPatchBytes), nil
	}
	if reason := checkPatch(kind, patch); reason != "" {
		return "patch rejected: " + reason + "\n", nil
	}

	data := []byte(patch)
	dryRun := metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}}
	if err := t.patch(ctx, kind, name, data, dryRun); err != nil {
		return fmt.Sprintf("patch rejected by the API server (dry run, nothing changed): %v\n", err), nil
	}
	if err := t.patch(ctx, kind, name, data, metav1.PatchOptions{}); err != nil { // actually apply the patch
		return fmt.Sprintf("patch failed after passing the dry run: %v\n", err), nil
	}
	t.applied = append(t.applied, AppliedPatch{Kind: kind, Name: name, Patch: patch})

	return t.describeAfterPatch(ctx, kind, name)
}

// checkPatch enforces the kind and field allowlists without calling the API,
// so a disallowed patch never reaches the cluster. It returns "" when allowed.
func checkPatch(kind, patch string) string {
	allowed, ok := allowedFields[kind]
	if !ok {
		return fmt.Sprintf("kind %q cannot be patched; only Deployment can", kind)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(patch), &top); err != nil {
		return fmt.Sprintf("patch is not a JSON object: %v", err)
	}
	if len(top) == 0 {
		return "patch is empty"
	}

	for field, raw := range top {
		subfields, ok := allowed[field]
		if !ok {
			return fmt.Sprintf("a %s patch may only change %s, not %q", kind, fieldList(allowed), field)
		}
		if subfields == nil {
			continue
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err != nil {
			return fmt.Sprintf("%q must be a JSON object", field)
		}
		for sub := range inner {
			if !contains(subfields, sub) {
				return fmt.Sprintf("a %s patch may only change %s, not %s.%s", kind, fieldList(allowed), field, sub)
			}
		}
	}
	return ""
}

// patch sends a strategic merge patch, which merges list items by key so a
// patch can change one container without restating the others.
func (t *Tools) patch(ctx context.Context, kind, name string, data []byte, opts metav1.PatchOptions) error {
	var err error
	switch kind {
	case "Deployment":
		_, err = t.client.AppsV1().Deployments(t.namespace).Patch(ctx, name, types.StrategicMergePatchType, data, opts)
	default:
		err = fmt.Errorf("kind %q cannot be patched", kind)
	}
	return err
}

// describeAfterPatch reads the object back after patching, so the model sees
// what actually changed, including surprises like a duplicated volume mount.
func (t *Tools) describeAfterPatch(ctx context.Context, kind, name string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "patched %s/%s. It now reads:\n", strings.ToLower(kind), name)

	d, err := t.client.AppsV1().Deployments(t.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("reading deployment %s after patching: %w", name, err)
	}
	writeDeployment(&b, *d)
	return b.String(), nil
}

// fieldList names the allowed fields for rejection messages, sorted so the
// same rejection always reads the same way.
func fieldList(allowed map[string][]string) string {
	var parts []string
	for field, subs := range allowed {
		if subs == nil {
			parts = append(parts, field)
			continue
		}
		for _, s := range subs {
			parts = append(parts, field+"."+s)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " and ")
}

// contains reports whether s is in list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
