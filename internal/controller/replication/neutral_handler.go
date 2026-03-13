// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"fmt"

	replicationv1alpha1 "github.com/ramendr/ramen/api/replication.storage.io/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NeutralAPIGroup is the API group for neutral replication
	NeutralAPIGroup = "replication.storage.io"

	// NeutralAPIVersion is the API version for neutral replication
	NeutralAPIVersion = "v1alpha1"
)

// NeutralHandler implements ReplicationHandler for the neutral replication.storage.io API
type NeutralHandler struct{}

// NewNeutralHandler creates a new NeutralHandler
func NewNeutralHandler() *NeutralHandler {
	return &NeutralHandler{}
}

// GetAPIGroup returns the neutral API group
func (h *NeutralHandler) GetAPIGroup() string {
	return NeutralAPIGroup
}

// GetAPIVersion returns the neutral API version
func (h *NeutralHandler) GetAPIVersion() string {
	return NeutralAPIVersion
}

// IsAvailable checks if the neutral API is available in the cluster
func (h *NeutralHandler) IsAvailable(ctx context.Context, c client.Client) (bool, error) {
	// Try to list VolumeGroupReplicationClasses to check if CRD exists
	vgrcList := &replicationv1alpha1.VolumeGroupReplicationClassList{}
	if err := c.List(ctx, vgrcList, &client.ListOptions{Limit: 1}); err != nil {
		if errors.IsNotFound(err) || errors.IsMethodNotSupported(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DiscoverVGRClasses discovers VolumeGroupReplicationClasses matching the criteria
func (h *NeutralHandler) DiscoverVGRClasses(
	ctx context.Context,
	c client.Client,
	storageClassName string,
	storageID string,
	schedule string,
) ([]VGRClassInfo, error) {
	vgrcList := &replicationv1alpha1.VolumeGroupReplicationClassList{}
	if err := c.List(ctx, vgrcList); err != nil {
		return nil, fmt.Errorf("failed to list VolumeGroupReplicationClasses: %w", err)
	}

	var classes []VGRClassInfo
	for i := range vgrcList.Items {
		vgrc := &vgrcList.Items[i]

		// Check storage ID label
		sid := vgrc.GetLabels()[StorageIDLabel]
		if sid != storageID {
			continue
		}

		// Check schedule parameter
		if schedule != "" {
			schedParam := vgrc.Spec.Parameters[ReplicationScheduleKey]
			if schedParam != schedule {
				continue
			}
		}

		// Extract IDs from labels
		rid := vgrc.GetLabels()[ReplicationIDLabel]
		grid := vgrc.GetLabels()[GroupReplicationIDLabel]

		classes = append(classes, VGRClassInfo{
			Name:               vgrc.Name,
			Provisioner:        vgrc.Spec.Provisioner,
			Parameters:         vgrc.Spec.Parameters,
			StorageID:          sid,
			ReplicationID:      rid,
			GroupReplicationID: grid,
			IsOffloaded:        false, // Can be determined from labels if needed
		})
	}

	return classes, nil
}

// CreateVGR creates a new VolumeGroupReplication using the neutral API
func (h *NeutralHandler) CreateVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	spec VGRSpec,
) error {
	vgr := &replicationv1alpha1.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: replicationv1alpha1.VolumeGroupReplicationSpec{
			ReplicationState:                replicationv1alpha1.ReplicationState(spec.ReplicationState),
			VolumeGroupReplicationClassName: spec.VGRClassName,
			AutoResync:                      spec.AutoResync,
			ReplicationHandle:               spec.ReplicationHandle,
		},
	}

	// Set source selector if provided
	if spec.PVCSelector != nil {
		vgr.Spec.Source = replicationv1alpha1.VolumeGroupReplicationSource{
			Selector: spec.PVCSelector,
		}
	}

	if err := c.Create(ctx, vgr); err != nil {
		return fmt.Errorf("failed to create VolumeGroupReplication: %w", err)
	}

	return nil
}

// GetVGR retrieves the status of a VolumeGroupReplication
func (h *NeutralHandler) GetVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (*VGRStatus, error) {
	vgr := &replicationv1alpha1.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	status := &VGRStatus{
		State:              State(vgr.Status.State),
		Message:            vgr.Status.Message,
		ObservedGeneration: vgr.Status.ObservedGeneration,
		LastSyncTime:       vgr.Status.LastSyncTime,
		LastSyncDuration:   vgr.Status.LastSyncDuration,
		LastSyncBytes:      vgr.Status.LastSyncBytes,
		Conditions:         vgr.Status.Conditions,
		PVCList:            vgr.Status.PersistentVolumeClaimsRefList,
		Ready:              isNeutralVGRReady(vgr),
	}

	return status, nil
}

// UpdateVGR updates an existing VolumeGroupReplication
func (h *NeutralHandler) UpdateVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	spec VGRSpec,
) error {
	vgr := &replicationv1alpha1.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	// Update spec fields
	vgr.Spec.ReplicationState = replicationv1alpha1.ReplicationState(spec.ReplicationState)
	vgr.Spec.VolumeGroupReplicationClassName = spec.VGRClassName
	vgr.Spec.AutoResync = spec.AutoResync

	if spec.PVCSelector != nil {
		vgr.Spec.Source.Selector = spec.PVCSelector
	}

	if err := c.Update(ctx, vgr); err != nil {
		return fmt.Errorf("failed to update VolumeGroupReplication: %w", err)
	}

	return nil
}

// DeleteVGR deletes a VolumeGroupReplication
func (h *NeutralHandler) DeleteVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) error {
	vgr := &replicationv1alpha1.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		if errors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	if err := c.Delete(ctx, vgr); err != nil {
		return fmt.Errorf("failed to delete VolumeGroupReplication: %w", err)
	}

	return nil
}

// IsVGRReady checks if a VolumeGroupReplication is ready
func (h *NeutralHandler) IsVGRReady(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (bool, error) {
	vgr := &replicationv1alpha1.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return false, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	return isNeutralVGRReady(vgr), nil
}

// GetVGRConditions retrieves the conditions of a VolumeGroupReplication
func (h *NeutralHandler) GetVGRConditions(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) ([]metav1.Condition, error) {
	vgr := &replicationv1alpha1.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	return vgr.Status.Conditions, nil
}

// isNeutralVGRReady determines if a VolumeGroupReplication is ready based on its status
func isNeutralVGRReady(vgr *replicationv1alpha1.VolumeGroupReplication) bool {
	// Check if state matches desired state
	desiredState := string(vgr.Spec.ReplicationState)
	currentState := string(vgr.Status.State)

	// Map states for comparison
	if desiredState == "primary" && currentState != "Primary" {
		return false
	}
	if desiredState == "secondary" && currentState != "Secondary" {
		return false
	}

	// Check conditions for any errors
	for _, cond := range vgr.Status.Conditions {
		if cond.Type == "Degraded" && cond.Status == metav1.ConditionTrue {
			return false
		}
		if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse {
			return false
		}
	}

	return true
}

// Ensure NeutralHandler implements ReplicationHandler
var _ ReplicationHandler = (*NeutralHandler)(nil)

// Made with Bob
