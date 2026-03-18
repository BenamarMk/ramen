// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// VolumeGroupReplicationInterface defines the common interface for VolumeGroupReplication objects
// This interface abstracts both volrep.VolumeGroupReplication and neutral.VolumeGroupReplication
type VolumeGroupReplicationInterface interface {
	client.Object
	GetSpec() VolumeGroupReplicationSpecInterface
	GetStatus() VolumeGroupReplicationStatusInterface
	SetSpec(VolumeGroupReplicationSpecInterface)
	SetStatus(VolumeGroupReplicationStatusInterface)
}

// VolumeGroupReplicationSpecInterface defines the common interface for VolumeGroupReplication spec
type VolumeGroupReplicationSpecInterface interface {
	GetReplicationState() ReplicationState
	GetVolumeGroupReplicationClassName() string
	GetVolumeGroupReplicationContentName() string
	GetSource() VolumeGroupReplicationSourceInterface
	GetAutoResync() bool
	GetReplicationHandle() string
	SetReplicationState(ReplicationState)
	SetVolumeGroupReplicationClassName(string)
	SetVolumeGroupReplicationContentName(string)
	SetAutoResync(bool)
	SetReplicationHandle(string)
}

// VolumeGroupReplicationSourceInterface defines the common interface for VolumeGroupReplication source
type VolumeGroupReplicationSourceInterface interface {
	GetSelector() *metav1.LabelSelector
	GetVolumeGroupReplicationContentName() *string
	SetSelector(*metav1.LabelSelector)
}

// VolumeGroupReplicationStatusInterface defines the common interface for VolumeGroupReplication status
type VolumeGroupReplicationStatusInterface interface {
	GetState() State
	GetMessage() string
	GetObservedGeneration() int64
	GetLastSyncTime() *metav1.Time
	GetLastSyncDuration() *metav1.Duration
	GetLastSyncBytes() *int64
	GetLastCompletionTime() *metav1.Time
	GetLastStartTime() *metav1.Time
	GetConditions() []metav1.Condition
	GetPersistentVolumeClaimsRefList() []corev1.LocalObjectReference
	SetState(State)
	SetMessage(string)
	SetObservedGeneration(int64)
	SetConditions([]metav1.Condition)
	SetPersistentVolumeClaimsRefList([]corev1.LocalObjectReference)
}

// VolumeGroupReplicationClassInterface defines the common interface for VolumeGroupReplicationClass objects
type VolumeGroupReplicationClassInterface interface {
	client.Object
	GetSpec() VolumeGroupReplicationClassSpecInterface
}

// VolumeGroupReplicationClassSpecInterface defines the common interface for VolumeGroupReplicationClass spec
type VolumeGroupReplicationClassSpecInterface interface {
	GetProvisioner() string
	GetParameters() map[string]string
}

// VolumeGroupReplicationContentInterface defines the common interface for VolumeGroupReplicationContent objects
type VolumeGroupReplicationContentInterface interface {
	client.Object
	GetSpec() VolumeGroupReplicationContentSpecInterface
	GetStatus() VolumeGroupReplicationContentStatusInterface
}

// VolumeGroupReplicationContentSpecInterface defines the common interface for VolumeGroupReplicationContent spec
type VolumeGroupReplicationContentSpecInterface interface {
	GetVolumeGroupReplicationRef() corev1.ObjectReference
	GetVolumeGroupReplicationClassName() string
	GetProvisioner() string
	GetParameters() map[string]string
}

// VolumeGroupReplicationContentStatusInterface defines the common interface for VolumeGroupReplicationContent status
type VolumeGroupReplicationContentStatusInterface interface {
	GetVolumeReplicationContentRefList() []corev1.LocalObjectReference
	GetCreationTime() *int64
	GetReadyToUse() *bool
}

// Made with Bob
