// Package tools exposes the Kubernetes operations the agent may call,
// scoped to a single namespace and formatted as compact text for the LLM.
package tools

import (
	"k8s.io/client-go/kubernetes"
)

// Toolbox gives the agent access to one namespace. The namespace is fixed at
// construction so the LLM can never point a tool at another namespace.
type Toolbox struct {
	client    kubernetes.Interface
	namespace string

	allowWrites bool           // off unless EnableWrites is called; diagnosis runs never write
	applied     []AppliedPatch // every patch that reached the cluster, for scoring the run
}

// New returns a Toolbox scoped to namespace.
func New(client kubernetes.Interface, namespace string) *Toolbox {
	return &Toolbox{client: client, namespace: namespace}
}

// EnableWrites turns on apply_patch. It is a separate,
// explicit call so read-only use stays the default.
func (t *Toolbox) EnableWrites() {
	t.allowWrites = true
}

// AppliedPatch records one change made to the cluster, so a run can be checked
// afterwards for patches the agent should not have made.
type AppliedPatch struct {
	Kind  string
	Name  string
	Patch string
}

// Applied returns a copy of the audit log, so callers can't alter the record.
func (t *Toolbox) Applied() []AppliedPatch {
	return append([]AppliedPatch(nil), t.applied...)
}
