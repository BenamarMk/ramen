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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FailoverClusterMap defines the clusters used for failover per subscription. Key is subscription name
type FailoverClusterMap map[string]string

// ApplicationVolumeReplicationSpec defines the desired state of ApplicationVolumeReplication
type ApplicationVolumeReplicationSpec struct {
	FailoverClusters FailoverClusterMap `json:"FailoverClusters,omitempty"`
	// S3 Endpoint to replicate PV metadata; this is for all VRGs.
	// The value of this field, will be progated to every VRG.
	// See VRG spec for more details.
	S3Endpoint string `json:"s3Endpoint"`

	// Name of k8s secret that contains the credentials to access the S3 endpoint.
	// If S3Endpoint is used, also specify the k8s secret that contains the S3
	// access key id and secret access key set using the keys: AWS_ACCESS_KEY_ID
	// and AWS_SECRET_ACCESS_KEY.  The value of this field, will be progated to every VRG.
	// See VRG spec for more details.
	S3SecretName string `json:"s3SecretName"`
}

// SubscriptionPlacementDecision lists each subscription with its home and peer clusters
type SubscriptionPlacementDecision struct {
	HomeCluster string `json:"homeCluster,omitempty"`
	PeerCluster string `json:"peerCluster,omitempty"`
}

// SubscriptionPlacementDecisionMap defines per subscription placement decision, key is subscription name
type SubscriptionPlacementDecisionMap map[string]*SubscriptionPlacementDecision

// ApplicationVolumeReplicationStatus defines the observed state of ApplicationVolumeReplication
type ApplicationVolumeReplicationStatus struct {
	// Decisions are for subscription and by AVR namespace (which is app namespace)
	Decisions SubscriptionPlacementDecisionMap `json:"Decisions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
