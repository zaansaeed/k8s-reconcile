// Package tools exposes the Kubernetes operations the agent may call,
// scoped to a single namespace and formatted as compact text for the LLM.
package tools

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Connect builds a cluster client from a kubeconfig, using the same lookup as
// kubectl: the given path if set, then $KUBECONFIG, then ~/.kube/config.
func Connect(kubeconfig string) (kubernetes.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = kubeconfig
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}
	return clientset, nil
}

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
