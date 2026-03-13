// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReplicationState represents the desired replication state
type ReplicationState string

const (
	// Primary promotes the volume group to primary
	Primary ReplicationState = "primary"

	// Secondary demotes the volume group to secondary
	Secondary ReplicationState = "secondary"

	// Resync triggers a resync operation
	Resync ReplicationState = "resync"
)

// State represents the current replication state
type State string

const (
	// PrimaryState indicates the volume group is in primary state
	PrimaryState State = "Primary"

	// SecondaryState indicates the volume group is in secondary state
	SecondaryState State = "Secondary"

	// UnknownState indicates the state is unknown
	UnknownState State = "Unknown"
)

// VGRSpec defines the specification for creating a VolumeGroupReplication
type VGRSpec struct {
	// ReplicationState is the desired replication state (primary, secondary, resync)
	ReplicationState ReplicationState

	// VGRClassName is the name of the VolumeGroupReplicationClass
	VGRClassName string

	// PVCSelector is the label selector for PVCs to replicate
	PVCSelector *metav1.LabelSelector

	// AutoResync enables automatic resync when in secondary state
	AutoResync bool

	// ReplicationHandle is an existing replication handle (optional)
	ReplicationHandle string
}

// VGRStatus represents the status of a VolumeGroupReplication
type VGRStatus struct {
	// State is the current replication state
	State State

	// Message contains any message from the storage system
	Message string

	// ObservedGeneration is the last generation reconciled
	ObservedGeneration int64

	// LastSyncTime is the time of the last successful sync
	LastSyncTime *metav1.Time

	// LastSyncDuration is the duration of the last sync
	LastSyncDuration *metav1.Duration

	// LastSyncBytes is the number of bytes transferred in the last sync
	LastSyncBytes *int64

	// Conditions represent the current conditions
	Conditions []metav1.Condition

	// PVCList is the list of PVCs in the volume group
	PVCList []corev1.LocalObjectReference

	// Ready indicates if the VGR is ready
	Ready bool
}

// VGRClassInfo contains information about a VolumeGroupReplicationClass
type VGRClassInfo struct {
	// Name is the name of the VGRClass
	Name string

	// Provisioner is the storage provisioner name
	Provisioner string

	// Parameters are the replication parameters
	Parameters map[string]string

	// StorageID is the storage identifier from labels
	StorageID string

	// ReplicationID is the replication identifier from labels
	ReplicationID string

	// GroupReplicationID is the group replication identifier from labels
	GroupReplicationID string

	// IsOffloaded indicates if replication is offloaded
	IsOffloaded bool
}

// ReplicationHandler defines the interface for volume group replication operations.
// This interface abstracts the differences between legacy (replication.storage.openshift.io)
// and neutral (replication.storage.io) APIs, enabling Ramen to work with both.
type ReplicationHandler interface {
	// GetAPIGroup returns the API group this handler manages
	// Returns: "replication.storage.openshift.io" for legacy, "replication.storage.io" for neutral
	GetAPIGroup() string

	// GetAPIVersion returns the API version
	// Returns: "v1alpha1" for both legacy and neutral
	GetAPIVersion() string

	// IsAvailable checks if the API is available in the cluster
	// This is used during discovery to determine which handler to use
	IsAvailable(ctx context.Context, client client.Client) (bool, error)

	// DiscoverVGRClasses discovers VolumeGroupReplicationClasses matching the criteria
	// Parameters:
	//   - storageClassName: The storage class to match
	//   - storageID: The storage ID to match (from labels)
	//   - schedule: The replication schedule to match (from parameters)
	// Returns: List of matching VGRClass information
	DiscoverVGRClasses(
		ctx context.Context,
		client client.Client,
		storageClassName string,
		storageID string,
		schedule string,
	) ([]VGRClassInfo, error)

	// CreateVGR creates a new VolumeGroupReplication resource
	// Parameters:
	//   - namespacedName: The name and namespace for the VGR
	//   - spec: The VGR specification
	// Returns: Error if creation fails
	CreateVGR(
		ctx context.Context,
		client client.Client,
		namespacedName types.NamespacedName,
		spec VGRSpec,
	) error

	// GetVGR retrieves the status of a VolumeGroupReplication
	// Parameters:
	//   - namespacedName: The name and namespace of the VGR
	// Returns: VGR status and error if retrieval fails
	GetVGR(
		ctx context.Context,
		client client.Client,
		namespacedName types.NamespacedName,
	) (*VGRStatus, error)

	// UpdateVGR updates an existing VolumeGroupReplication
	// Parameters:
	//   - namespacedName: The name and namespace of the VGR
	//   - spec: The updated VGR specification
	// Returns: Error if update fails
	UpdateVGR(
		ctx context.Context,
		client client.Client,
		namespacedName types.NamespacedName,
		spec VGRSpec,
	) error

	// DeleteVGR deletes a VolumeGroupReplication
	// Parameters:
	//   - namespacedName: The name and namespace of the VGR
	// Returns: Error if deletion fails
	DeleteVGR(
		ctx context.Context,
		client client.Client,
		namespacedName types.NamespacedName,
	) error

	// IsVGRReady checks if a VolumeGroupReplication is ready
	// Parameters:
	//   - namespacedName: The name and namespace of the VGR
	// Returns: true if ready, false otherwise, and error if check fails
	IsVGRReady(
		ctx context.Context,
		client client.Client,
		namespacedName types.NamespacedName,
	) (bool, error)

	// GetVGRConditions retrieves the conditions of a VolumeGroupReplication
	// Parameters:
	//   - namespacedName: The name and namespace of the VGR
	// Returns: List of conditions and error if retrieval fails
	GetVGRConditions(
		ctx context.Context,
		client client.Client,
		namespacedName types.NamespacedName,
	) ([]metav1.Condition, error)
}

// HandlerType represents the type of replication handler
type HandlerType string

const (
	// LegacyHandlerType represents the legacy csi-addons handler
	LegacyHandlerType HandlerType = "legacy"

	// NeutralHandlerType represents the neutral replication.storage.io handler
	NeutralHandlerType HandlerType = "neutral"
)

// Made with Bob
