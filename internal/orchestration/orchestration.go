// Package orchestration deploys and supervises a local Kubernetes cluster
// (kind) on the agent host. It runs entirely agent-side: the client only drives
// it through gRPC, so a local Linux host and a remote Linux host take the exact
// same code path.
//
// The package depends only on the container engine, shell execution and systemd
// unit management. It must not import internal/db, internal/config or
// internal/command, so the orchestration logic stays a reusable agent building
// block (the agent coordinator wires it to the gRPC layer).
package orchestration

import (
	"github.com/amadeusitgroup/cds/internal/bo"
)

const (
	// ClusterName is the fixed kind cluster name CDS manages on a host.
	ClusterName = "cds-kind"
	// ControlPlaneNode is the podman container kind creates for the cluster's
	// control plane; it is derived from the cluster name by kind.
	ControlPlaneNode = ClusterName + "-control-plane"
)

// ProgressReporter receives human-readable progress during a long-running
// cluster operation (e.g. BuildCluster). It mirrors the container deploy
// reporter so the gRPC layer can stream the same way.
type ProgressReporter func(phase, message string) error

// Cluster is a point-in-time view of the managed cluster on a host.
type Cluster struct {
	// Name is the kind cluster name (always ClusterName for now).
	Name string
	// Node is the control-plane container name.
	Node string
	// Status is the live status of the control-plane node, derived from the
	// engine (podman inspect), not from any stored state.
	Status bo.ContainerStatus
}

// ClusterProvider abstracts the orchestration engine behind the cluster
// lifecycle. kind is the only implementation today, but the seam is kept: the
// agent selects a provider by engine name, so another provider (k3s, a remote
// cluster, ...) can be added without touching the service layer.
type ClusterProvider interface {
	// Name is the provider's engine name, e.g. "kind".
	Name() string
	// Build creates the cluster if it is not already present, streaming
	// progress. configFile is an optional provider-specific config path (empty
	// for the default). Build is idempotent.
	Build(configFile string, report ProgressReporter) error
	// Start starts an existing, stopped cluster.
	Start() error
	// Stop stops a running cluster without deleting it.
	Stop() error
	// Delete removes the cluster. When force is false and the cluster is in
	// use, Delete may refuse; when true it tears the cluster down regardless.
	Delete(force bool) error
	// Status returns the live cluster view.
	Status() (Cluster, error)
	// Kubeconfig returns the cluster's kubeconfig (minified, flattened) as read
	// from the control-plane node.
	Kubeconfig() (string, error)
}
