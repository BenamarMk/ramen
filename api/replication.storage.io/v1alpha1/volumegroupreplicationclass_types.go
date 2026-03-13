// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolumeGroupReplicationClassSpec defines the desired state of VolumeGroupReplicationClass
type VolumeGroupReplicationClassSpec struct {
	// provisioner is the name of storage provisioner
	// +kubebuilder:validation:Required
	Provisioner string `json:"provisioner"`

	// parameters is a key-value map with storage provisioner specific configurations for
	// creating volume replications
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vgrclass
// +kubebuilder:printcolumn:name="Provisioner",type=string,JSONPath=`.spec.provisioner`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VolumeGroupReplicationClass is the Schema for the volumegroupreplicationclasses API
type VolumeGroupReplicationClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VolumeGroupReplicationClassSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// VolumeGroupReplicationClassList contains a list of VolumeGroupReplicationClass
type VolumeGroupReplicationClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeGroupReplicationClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&VolumeGroupReplicationClass{},
		&VolumeGroupReplicationClassList{},
	)
}

// Made with Bob
