# K8s-Reconcile

A multi-agent LLM system that diagnoses and fixes broken Kubernetes deployments.

A triage orchestrator inspects a failing cluster and routes each fault to a specialist
agent (config, resources, networking), which investigates with Kubernetes API tools,
applies a fix, and verifies it. Every LLM call is logged, so each fix has a measured
token and dollar cost.

The system is benchmarked on ~20 injected faults across 4 difficulty tiers, from a bad
image tag to cascading multi-service failures, against a rule-based baseline and a
single-agent baseline.

## Stack

Go · client-go · kind (local Kubernetes in Docker) · Claude API

## Status

Early development: the cluster connection and listing of nodes and pods work.

## Run

Requires Docker, kind, and Go.

    kind create cluster
    go run ./cmd/reconcile -namespace=default