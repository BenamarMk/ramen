// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DryRunAnnotation carries the lifecycle phase of a test failover on the DRPC.
// Only Ramen writes it. It is present for as long as a test copy exists, and
// is removed once the copy has been torn down -- which is what gates a second
// run on the first one being finished with.
const DryRunAnnotation = "drplacementcontrol.ramendr.openshift.io/dry-run"

// DryRunPhase is the hub-side lifecycle of a test failover, carried in
// DryRunAnnotation. Absent means no run is in progress and a new one may begin.
//
// Three of the four values are gerunds naming what Ramen is doing. Ready is an
// adjective naming a resulting state, because it is the one phase in which
// Ramen has stopped and nothing further happens without the user.
//
// The Enum marker below documents the values; it does not enforce them. This
// type is never a struct field -- the phase lives in an annotation -- so
// controller-gen emits it into no CRD schema and the API server validates
// nothing. Only Ramen writes the annotation, so there is nothing to enforce.
// +kubebuilder:validation:Enum=SettingUp;Reverting;Ready;TearingDown
type DryRunPhase string

const (
	// DryRunSettingUp means the test cluster has been promoted and the test
	// copy is being built -- or has stalled while building.
	DryRunSettingUp = DryRunPhase("SettingUp")

	// DryRunReverting means the test copy is finished or has been abandoned,
	// and the test cluster is being returned to Secondary.
	DryRunReverting = DryRunPhase("Reverting")

	// DryRunReady means the test copy is built and running, and Ramen is done:
	// the cluster is back to Secondary and nothing further happens until the
	// user clears spec.dryRun, which asks for the copy to be taken down.
	//
	// Deliberately not "Completed". The orchestration has completed, but the
	// test has not -- the copy runs until someone takes it down, which is the
	// whole point. "Completed" also already means two other things here:
	// ProgressionCompleted sits in the adjacent printer column, and
	// DryRunBuildCompleted is status.dryRun.state.
	DryRunReady = DryRunPhase("Ready")

	// DryRunTearingDown means the user has cleared spec.dryRun and the test
	// copy is being deleted. The annotation is removed once it is gone, which
	// is what frees the DRPC to run another test.
	DryRunTearingDown = DryRunPhase("TearingDown")
)

// DryRunState is how far the cluster building the test copy has got.
// +kubebuilder:validation:Enum=InProgress;Completed
type DryRunState string

const (
	// DryRunBuildInProgress means the test copy is not ready yet. There is
	// deliberately no failure state: anything that cannot proceed stays
	// InProgress, records why in Message, and requeues until a human
	// intervenes. Nothing times out and nothing is cleaned up automatically.
	DryRunBuildInProgress = DryRunState("InProgress")

	// DryRunBuildCompleted means every test namespace was created, every
	// test PVC is Bound, and every kube object restore finished.
	DryRunBuildCompleted = DryRunState("Completed")
)

// DryRunStatus reports a test failover. The VRG on the cluster building the
// test copy publishes it, and the DRPC mirrors it, because the managed
// cluster's log is not somewhere a DRPC user looks.
type DryRunStatus struct {
	// runID identifies this test run and is the suffix of every test namespace
	// it creates.
	//+optional
	RunID string `json:"runID,omitempty"`

	// namespaceMapping maps each protected namespace to the test namespace its
	// contents were restored into. This is where a user finds the test copy.
	//+optional
	NamespaceMapping map[string]string `json:"namespaceMapping,omitempty"`

	//+optional
	State DryRunState `json:"state,omitempty"`

	//+optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	//+optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// message explains why a run that is still InProgress has not advanced.
	// With no timeout and no failure state, this is the only signal a stalled
	// run gives, so it must name the resource being waited on.
	//+optional
	Message string `json:"message,omitempty"`

	// targetCluster is the peer promoted to build the test copy. It is derived
	// once when the run starts and read back thereafter, so the run cannot
	// retarget itself if cluster roles shift underneath it.
	// Set on the DRPC only.
	//+optional
	TargetCluster string `json:"targetCluster,omitempty"`

	// actionAtStart and failoverClusterAtStart record the spec the run began
	// against. A change in either is a real DR request, which pre-empts the
	// test. Both are watched because action alone is not enough: a DRPC
	// already holding Failover from an earlier real failover shows no change
	// when the user retargets failoverCluster at a genuine disaster.
	// Set on the DRPC only.
	//+optional
	ActionAtStart DRAction `json:"actionAtStart,omitempty"`

	//+optional
	FailoverClusterAtStart string `json:"failoverClusterAtStart,omitempty"`
}
