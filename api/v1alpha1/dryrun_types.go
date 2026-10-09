// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DryRunAnnotation carries the lifecycle phase of a test failover on the DRPC.
// Only Ramen writes it, and Ramen never clears it: leaving the terminal phase
// is a deliberate user action, and is the only gate on starting a second run.
const DryRunAnnotation = "drplacementcontrol.ramendr.openshift.io/dry-run"

// DryRunPhase is the hub-side lifecycle of a test failover, carried in
// DryRunAnnotation. Absent means no run is in progress and a new one may begin.
// +kubebuilder:validation:Enum=SettingUp;Reverting;Completed
type DryRunPhase string

const (
	// DryRunSettingUp means the test cluster has been promoted and the test
	// copy is being built -- or has stalled while building.
	DryRunSettingUp = DryRunPhase("SettingUp")

	// DryRunReverting means the test copy is finished or has been abandoned,
	// and the test cluster is being returned to Secondary.
	DryRunReverting = DryRunPhase("Reverting")

	// DryRunCompleted is terminal. The cycle is done and the test copy is left
	// running for the user to inspect and eventually delete.
	DryRunCompleted = DryRunPhase("Completed")
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
