// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReplicationState represents the replication operations to be performed on the volume group
type ReplicationState string

const (
	// Primary promotes the volume group to primary
	Primary ReplicationState = "primary"

	// Secondary demotes the volume group to secondary
	Secondary ReplicationState = "secondary"

	// Resync triggers a resync operation on the volume group
	Resync ReplicationState = "resync"
)

// State represents the state of the volume group replication
type State string

const (
	// PrimaryState represents the Primary replication state
	PrimaryState State = "Primary"

	// SecondaryState represents the Secondary replication state
	SecondaryState State = "Secondary"

	// UnknownState represents the Unknown replication state
	UnknownState State = "Unknown"
)

// VolumeGroupReplicationContentHandle represents the CSI volume group snapshot handle
type VolumeGroupReplicationContentHandle string

// VolumeReplicationContentHandle represents the CSI volume snapshot handle
type VolumeReplicationContentHandle string

// SecretReference contains the reference to a secret
type SecretReference struct {
	// Name is the name of the secret
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the secret
	Namespace string `json:"namespace,omitempty"`
}

// LastSyncTime is the time of the last successful synchronization
type LastSyncTime struct {
	// Time is the time of the last successful synchronization
	// +optional
	Time *metav1.Time `json:"time,omitempty"`
}

// LastSyncDuration is the duration of the last successful synchronization
type LastSyncDuration struct {
	// Duration is the duration of the last successful synchronization
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`
}

// LastSyncBytes is the number of bytes transferred during the last successful synchronization
type LastSyncBytes struct {
	// Bytes is the number of bytes transferred during the last successful synchronization
	// +optional
	Bytes *int64 `json:"bytes,omitempty"`
}

// VolumeReplicationClass is the Schema for the volumereplicationclasses API
// This is a reference type used in VolumeGroupReplicationClass
type VolumeReplicationClassRef struct {
	// Name of the VolumeReplicationClass
	Name string `json:"name"`
}

// PersistentVolumeClaimSpec describes the common attributes of storage devices
// and allows a Source for provider-specific attributes
type PersistentVolumeClaimSpec struct {
	// accessModes contains the desired access modes the volume should have.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#access-modes-1
	// +optional
	// +listType=atomic
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// selector is a label query over volumes to consider for binding.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// resources represents the minimum resources the volume should have.
	// If RecoverVolumeExpansionFailure feature is enabled users are allowed to specify resource requirements
	// that are lower than previous value but must still be higher than capacity recorded in the
	// status field of the claim.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#resources
	// +optional
	Resources corev1.VolumeResourceRequirements `json:"resources,omitempty"`

	// volumeName is the binding reference to the PersistentVolume backing this claim.
	// +optional
	VolumeName string `json:"volumeName,omitempty"`

	// storageClassName is the name of the StorageClass required by the claim.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// volumeMode defines what type of volume is required by the claim.
	// Value of Filesystem is implied when not included in claim spec.
	// +optional
	VolumeMode *corev1.PersistentVolumeMode `json:"volumeMode,omitempty"`

	// dataSource field can be used to specify either:
	// * An existing VolumeSnapshot object (snapshot.storage.k8s.io/VolumeSnapshot)
	// * An existing PVC (PersistentVolumeClaim)
	// If the provisioner or an external controller can support the specified data source,
	// it will create a new volume based on the contents of the specified data source.
	// When the AnyVolumeDataSource feature gate is enabled, dataSource contents will be copied to dataSourceRef,
	// and dataSourceRef contents will be copied to dataSource when dataSourceRef.namespace is not specified.
	// If the namespace is specified, then dataSourceRef will not be copied to dataSource.
	// +optional
	DataSource *corev1.TypedLocalObjectReference `json:"dataSource,omitempty"`

	// dataSourceRef specifies the object from which to populate the volume with data, if a non-empty
	// volume is desired. This may be any object from a non-empty API group (non
	// core object) or a PersistentVolumeClaim object.
	// When this field is specified, volume binding will only succeed if the type of
	// the specified object matches some installed volume populator or dynamic
	// provisioner.
	// This field will replace the functionality of the dataSource field and as such
	// if both fields are non-empty, they must have the same value. For backwards
	// compatibility, when namespace isn't specified in dataSourceRef,
	// both fields (dataSource and dataSourceRef) will be set to the same
	// value automatically if one of them is empty and the other is non-empty.
	// When namespace is specified in dataSourceRef,
	// dataSource isn't set to the same value and must be empty.
	// There are three important differences between dataSource and dataSourceRef:
	// * While dataSource only allows two specific types of objects, dataSourceRef
	//   allows any non-core object, as well as PersistentVolumeClaim objects.
	// * While dataSource ignores disallowed values (dropping them), dataSourceRef
	//   preserves all values, and generates an error if a disallowed value is
	//   specified.
	// * While dataSource only allows local objects, dataSourceRef allows objects
	//   in any namespaces.
	// (Beta) Using this field requires the AnyVolumeDataSource feature gate to be enabled.
	// (Alpha) Using the namespace field of dataSourceRef requires the CrossNamespaceVolumeDataSource feature gate to be enabled.
	// +optional
	DataSourceRef *corev1.TypedObjectReference `json:"dataSourceRef,omitempty"`
}

// Made with Bob
