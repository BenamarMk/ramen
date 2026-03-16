// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolumeGroupReplicationSpec defines the desired state of VolumeGroupReplication
type VolumeGroupReplicationSpec struct {
	// replicationState represents the replication operation to be performed on the volume group.
	// Supported operations are "primary", "secondary" and "resync"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=primary;secondary;resync
	ReplicationState ReplicationState `json:"replicationState"`

	// volumeGroupReplicationClassName is the name of the VolumeGroupReplicationClass
	// +kubebuilder:validation:Required
	VolumeGroupReplicationClassName string `json:"volumeGroupReplicationClassName"`

	// volumeGroupReplicationContentName is the name of the VolumeGroupReplicationContent object
	// +optional
	VolumeGroupReplicationContentName string `json:"volumeGroupReplicationContentName,omitempty"`

	// source specifies where a group snapshot will be created from.
	// This field is immutable after creation.
	// Required.
	// +kubebuilder:validation:Required
	Source VolumeGroupReplicationSource `json:"source"`

	// autoResync represents the group to be auto resynced when
	// ReplicationState is "secondary"
	// +optional
	AutoResync bool `json:"autoResync,omitempty"`

	// replicationHandle represents an existing (but new) replication id
	// +optional
	ReplicationHandle string `json:"replicationHandle,omitempty"`
}

// VolumeGroupReplicationSource specifies whether the snapshot is (or should be)
// dynamically provisioned or already exists, and just requires a
// Kubernetes object representation. Exactly one of its members must be set.
type VolumeGroupReplicationSource struct {
	// selector is a label query over persistent volume claims that are to be
	// grouped together for replication.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// volumeReplicationContentName specifies the name of a pre-existing VolumeReplicationContent
	// object representing an existing volume group snapshot.
	// This field should be set if the volume group snapshot already exists and
	// only needs a representation in Kubernetes.
	// This field is immutable.
	// +optional
	VolumeGroupReplicationContentName *string `json:"volumeGroupReplicationContentName,omitempty"`
}

// VolumeGroupReplicationStatus defines the observed state of VolumeGroupReplication
type VolumeGroupReplicationStatus struct {
	// state captures the latest state of the replication operation.
	// +optional
	State State `json:"state,omitempty"`

	// message contains any message from the underlying storage system
	// +optional
	Message string `json:"message,omitempty"`

	// observedGeneration is the last generation change the operator has dealt with
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastSyncTime is the time of the last successful synchronization.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// lastSyncDuration is the duration of the last successful synchronization.
	// +optional
	LastSyncDuration *metav1.Duration `json:"lastSyncDuration,omitempty"`

	// lastSyncBytes is the number of bytes transferred during the last successful synchronization.
	// +optional
	LastSyncBytes *int64 `json:"lastSyncBytes,omitempty"`

	// lastCompletionTime is the time of the last successful synchronization.
	// +optional
	LastCompletionTime *metav1.Time `json:"lastCompletionTime,omitempty"`

	// lastStartTime is the time the last synchronization started.
	// +optional
	LastStartTime *metav1.Time `json:"lastStartTime,omitempty"`

	// conditions represent the latest available observations of the
	// volume group replication's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// persistentVolumeClaimsRefList is the list of PVCs for the volume group replication.
	// The maximum number of allowed PVCs in the group is 100.
	// +optional
	PersistentVolumeClaimsRefList []corev1.LocalObjectReference `json:"persistentVolumeClaimsRefList,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=vgr
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ReplicationState",type=string,JSONPath=`.spec.replicationState`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`

// VolumeGroupReplication is the Schema for the volumegroupreplications API
type VolumeGroupReplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeGroupReplicationSpec   `json:"spec,omitempty"`
	Status VolumeGroupReplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VolumeGroupReplicationList contains a list of VolumeGroupReplication
type VolumeGroupReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeGroupReplication `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vgrc
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ReadyToUse",type=boolean,JSONPath=`.status.readyToUse`

// VolumeGroupReplicationContent represents the actual "on-disk" group snapshot object
// in the underlying storage system
type VolumeGroupReplicationContent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeGroupReplicationContentSpec   `json:"spec,omitempty"`
	Status VolumeGroupReplicationContentStatus `json:"status,omitempty"`
}

// VolumeGroupReplicationContentSpec defines the desired state of VolumeGroupReplicationContent
type VolumeGroupReplicationContentSpec struct {
	// volumeGroupReplicationRef specifies the VolumeGroupReplication object to which this
	// VolumeGroupReplicationContent object is bound.
	// VolumeGroupReplication.Spec.VolumeGroupReplicationContentName field must reference to
	// this VolumeGroupReplicationContent's name for the bidirectional binding to be valid.
	// For a pre-existing VolumeGroupReplicationContent object, name and namespace of the
	// VolumeGroupReplication object MUST be provided for binding to happen.
	// This field is immutable after creation.
	// Required.
	// +kubebuilder:validation:Required
	VolumeGroupReplicationRef corev1.ObjectReference `json:"volumeGroupReplicationRef"`

	// volumeGroupReplicationClassName is the name of the VolumeGroupReplicationClass from
	// which this group snapshot was (or will be) created.
	// +kubebuilder:validation:Required
	VolumeGroupReplicationClassName string `json:"volumeGroupReplicationClassName"`

	// source specifies whether the snapshot is (or should be) dynamically provisioned
	// or already exists, and just requires a Kubernetes object representation.
	// This field is immutable after creation.
	// Required.
	// +kubebuilder:validation:Required
	Source VolumeGroupReplicationContentSource `json:"source"`

	// provisioner is the name of the CSI driver used to create the physical
	// volume group snapshot on the underlying storage system.
	// This MUST be the same as the name returned by the CSI GetPluginName() call for
	// that driver.
	// Required.
	// +kubebuilder:validation:Required
	Provisioner string `json:"provisioner"`

	// volumeGroupReplicationHandle is a unique id returned by the CSI driver
	// to identify the VolumeGroupReplication on the storage system.
	// +optional
	VolumeGroupReplicationHandle *VolumeGroupReplicationContentHandle `json:"volumeGroupReplicationHandle,omitempty"`

	// parameters is a key-value map with storage driver specific parameters for creating replications.
	// These values are opaque to Kubernetes.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// VolumeGroupReplicationContentSource represents the CSI source of a group snapshot.
type VolumeGroupReplicationContentSource struct {
	// volumeHandles is a list of volume handles on the backend to be replicated.
	// +optional
	VolumeHandles []string `json:"volumeHandles,omitempty"`
}

// VolumeGroupReplicationContentStatus defines the status of VolumeGroupReplicationContent
type VolumeGroupReplicationContentStatus struct {
	// volumeGroupReplicationHandle is a unique id returned by the CSI driver
	// to identify the VolumeGroupReplication on the storage system.
	// If a storage system does not provide such an id, the
	// CSI driver can choose to return the VolumeGroupReplication name.
	// +optional
	VolumeGroupReplicationHandle *VolumeGroupReplicationContentHandle `json:"volumeGroupReplicationHandle,omitempty"`

	// volumeReplicationContentRefList is the list of volume replication content references for this group replication.
	// The maximum number of allowed volume replication contents in the group is 100.
	// +optional
	VolumeReplicationContentRefList []corev1.LocalObjectReference `json:"volumeReplicationContentRefList,omitempty"`

	// creationTime is the timestamp when the point-in-time group snapshot is taken
	// by the underlying storage system.
	// If not specified, it indicates the creation time is unknown.
	// If not specified, it means the readiness of a group snapshot is unknown.
	// The format of this field is a Unix nanoseconds time encoded as an int64.
	// On Unix, the command date +%s%N returns the current time in nanoseconds
	// since 1970-01-01 00:00:00 UTC.
	// +optional
	CreationTime *int64 `json:"creationTime,omitempty"`

	// readyToUse indicates if all the individual snapshots in the group are ready
	// to be used to restore a group of volumes.
	// ReadyToUse becomes true when ReadyToUse of all individual snapshots become true.
	// If not specified, it means the readiness of a group snapshot is unknown.
	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// error is the last observed error during group snapshot creation, if any.
	// Upon success after retry, this error field will be cleared.
	// +optional
	Error *VolumeGroupReplicationError `json:"error,omitempty"`

	// persistentVolumeReplicationContentRefList is the list of PV replication content references for this group replication.
	// +optional
	PersistentVolumeReplicationContentRefList []corev1.LocalObjectReference `json:"persistentVolumeReplicationContentRefList,omitempty"`
}

// VolumeGroupReplicationError describes an error encountered during group snapshot creation.
type VolumeGroupReplicationError struct {
	// time is the timestamp when the error was encountered.
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// message is a string detailing the encountered error during snapshot
	// creation if specified.
	// NOTE: message may be logged, and it should not contain sensitive
	// information.
	// +optional
	Message *string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vgrc
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ReadyToUse",type=boolean,JSONPath=`.status.readyToUse`

// VolumeGroupReplicationContentList contains a list of VolumeGroupReplicationContent
type VolumeGroupReplicationContentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeGroupReplicationContent `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&VolumeGroupReplication{},
		&VolumeGroupReplicationList{},
		&VolumeGroupReplicationContent{},
		&VolumeGroupReplicationContentList{},
	)
}

// Made with Bob
