// Package tools exposes the Kubernetes operations the agent may call,
// scoped to a single namespace and formatted as compact text for the LLM.
package tools

import (
	"k8s.io/client-go/kubernetes"
)

// Tools gives the agent access to one namespace. The namespace is fixed at
// construction so the LLM can never point a tool at another namespace.
type Tools struct {
	client    kubernetes.Interface
	namespace string
}

// New returns Tools scoped to namespace.
func New(client kubernetes.Interface, namespace string) *Tools {
	return &Tools{client: client, namespace: namespace}
}
