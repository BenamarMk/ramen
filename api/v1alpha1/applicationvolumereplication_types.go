/*
Copyright 2021 The RamenDR authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	plrv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DRAction which will be either a failover or failback action
// +kubebuilder:validation:Enum=Failover;Failback
type DRAction string

// These are the valid values for DRAction
const (
	// Failover, restore PVs to the TargetCluster
	ActionFailover DRAction = "Failover"

	// Failback, restore PVs to the PreferredCluster
	ActionFailback DRAction = "Failback"

	// Relocate, restore PVs to the designated TargetCluster.  PreferredCluster will change
	// to be the TargetCluster.
	ActionRelocate DRAction = "Relocate"
)

// DRClusterPeersReference holds a reference to DRClusterPeers
type DRClusterPeersReference struct {
	// `namespace` is the namespace of the cluster peers.
	Namespace string `json:"namespace,omitempty"`
	// `name` is the name of the cluster peers.
	Name string `json:"name"`
}

// ApplicationVolumeReplicationSpec defines the desired state of ApplicationVolumeReplication
type ApplicationVolumeReplicationSpec struct {
	// Placement is used by AVR
	Placement *plrv1.Placement `json:"placement"`

	// DRClusterPeersRef is the reference to the DRClusterPeers participating in the DR replication for this AVR
	DRClusterPeersRef DRClusterPeersReference `json:"drClusterPeersRef"`

	// PreferredCluster is the cluster name that the user preferred to run the application on
	PreferredCluster string `json:"preferredCluster,omitempty"`

	// FailoverCluster is the cluster name that the user wants to failover the application to.
	// If not sepcified, then the AVR will select the surviving cluster from the DRClusterPeers
	FailoverCluster string `json:"failoverCluster,omitempty"`

	// Label selector to identify all the PVCs that need DR protection.
	// This selector is assumed to be the same for all subscriptions that
	// need DR protection. It will be passed in to the VRG when it is created
	PVCSelector metav1.LabelSelector `json:"pvcSelector"`

	// S3 Endpoint to replicate PV metadata; this is for all VRGs.
	// The value of this field, will be progated to every VRG.
	// See VRG spec for more details.
	S3Endpoint string `json:"s3Endpoint"`

	// S3 Region: https://docs.aws.amazon.com/general/latest/gr/rande.html
	S3Region string `json:"s3Region"`

	// Name of k8s secret that contains the credentials to access the S3 endpoint.
	// If S3Endpoint is used, also specify the k8s secret that contains the S3
	// access key id and secret access key set using the keys: AWS_ACCESS_KEY_ID
	// and AWS_SECRET_ACCESS_KEY.  The value of this field, will be progated to every VRG.
	// See VRG spec for more details.
	S3SecretName string `json:"s3SecretName"`

	// Action is either failover or failback operation
	Action DRAction `json:"action,omitempty"`
}

// DRState for keeping track of the DR placement
// +kubebuilder:validation:Enum=Initial;Failing-over;Failed-over;Failing-back;Failed-back
type DRState string

// These are the valid values for DRState
const (
	// Initial, this is the state that will be recorded in the AVR status
	// when initial deplyment has been performed successfully
	Initial DRState = "Initial"

	// FailingOver, state recorded in the AVR status when the failover
	// is initiated but has not been completed yet
	FailingOver DRState = "Failing-over"

	// FailedOver, state recorded in the AVR status when the failover
	// process has completed
	FailedOver DRState = "Failed-over"

	// FailingBack, state recorded in the AVR status when the failback
	// is initiated but has not been completed yet
	FailingBack DRState = "Failing-back"

	// FailedBack, state recorded in the AVR status when the failback
	// process has completed
	FailedBack DRState = "Failed-back"

	Relocating DRState = "Relocating"

	Relocated DRState = "Relocated"
)

// PlacementDecision holds references to the current successful placement
type PlacementDecision struct {
	HomeCluster          string `json:"homeCluster,omitempty"`
	PeerCluster          string `json:"peerCluster,omitempty"`
	PreferredHomeCluster string `json:"preferredHomeCluster,omitempty"`
}

// ApplicationVolumeReplicationStatus defines the observed state of ApplicationVolumeReplication
type ApplicationVolumeReplicationStatus struct {
	Decision         PlacementDecision `json:"decisions,omitempty"`
	LastKnownDRState DRState           `json:"lastKnownDRState,omitempty"`
	LastUpdateTime   metav1.Time       `json:"lastUpdateTime"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=avr

// ApplicationVolumeReplication is the Schema for the applicationvolumereplications API
type ApplicationVolumeReplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationVolumeReplicationSpec   `json:"spec,omitempty"`
	Status ApplicationVolumeReplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationVolumeReplicationList contains a list of ApplicationVolumeReplication
type ApplicationVolumeReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationVolumeReplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationVolumeReplication{}, &ApplicationVolumeReplicationList{})
}
