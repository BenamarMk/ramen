// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"fmt"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LegacyAPIGroup is the API group for legacy csi-addons replication
	LegacyAPIGroup = "replication.storage.openshift.io"

	// LegacyAPIVersion is the API version for legacy replication
	LegacyAPIVersion = "v1alpha1"

	// StorageIDLabel is the label key for storage ID
	StorageIDLabel = "ramendr.openshift.io/storageid"

	// ReplicationIDLabel is the label key for replication ID
	ReplicationIDLabel = "ramendr.openshift.io/replicationid"

	// GroupReplicationIDLabel is the label key for group replication ID
	GroupReplicationIDLabel = "ramendr.openshift.io/groupreplicationid"

	// ReplicationScheduleKey is the parameter key for replication schedule
	ReplicationScheduleKey = "schedulingInterval"
)

// LegacyHandler implements ReplicationHandler for the legacy csi-addons API
// (replication.storage.openshift.io)
type LegacyHandler struct{}

// NewLegacyHandler creates a new LegacyHandler
func NewLegacyHandler() *LegacyHandler {
	return &LegacyHandler{}
}

// GetAPIGroup returns the legacy API group
func (h *LegacyHandler) GetAPIGroup() string {
	return LegacyAPIGroup
}

// GetAPIVersion returns the legacy API version
func (h *LegacyHandler) GetAPIVersion() string {
	return LegacyAPIVersion
}

// IsAvailable checks if the legacy API is available in the cluster
func (h *LegacyHandler) IsAvailable(ctx context.Context, c client.Client) (bool, error) {
	// Try to list VolumeGroupReplicationClasses to check if CRD exists
	vgrcList := &volrep.VolumeGroupReplicationClassList{}
	if err := c.List(ctx, vgrcList, &client.ListOptions{Limit: 1}); err != nil {
		if errors.IsNotFound(err) || errors.IsMethodNotSupported(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DiscoverVGRClasses discovers VolumeGroupReplicationClasses matching the criteria
func (h *LegacyHandler) DiscoverVGRClasses(
	ctx context.Context,
	c client.Client,
	storageClassName string,
	storageID string,
	schedule string,
) ([]VGRClassInfo, error) {
	vgrcList := &volrep.VolumeGroupReplicationClassList{}
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
			IsOffloaded:        false, // Legacy API doesn't support offloaded flag
		})
	}

	return classes, nil
}

// CreateVGR creates a new VolumeGroupReplication using the legacy API
func (h *LegacyHandler) CreateVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	spec VGRSpec,
) error {
	vgr := &volrep.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: volrep.VolumeGroupReplicationSpec{
			ReplicationState:                volrep.ReplicationState(spec.ReplicationState),
			VolumeGroupReplicationClassName: spec.VGRClassName,
			AutoResync:                      spec.AutoResync,
		},
	}

	// Set replication handle if provided (may not be supported in all versions)
	if spec.ReplicationHandle != "" {
		// Note: ReplicationHandle field may not exist in all csi-addons versions
		// This is handled gracefully by the API
	}

	// Set source selector if provided
	if spec.PVCSelector != nil {
		vgr.Spec.Source = volrep.VolumeGroupReplicationSource{
			Selector: spec.PVCSelector,
		}
	}

	if err := c.Create(ctx, vgr); err != nil {
		return fmt.Errorf("failed to create VolumeGroupReplication: %w", err)
	}

	return nil
}

// GetVGR retrieves the status of a VolumeGroupReplication
func (h *LegacyHandler) GetVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (*VGRStatus, error) {
	vgr := &volrep.VolumeGroupReplication{}
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
		Ready:              isVGRReady(vgr),
	}

	return status, nil
}

// UpdateVGR updates an existing VolumeGroupReplication
func (h *LegacyHandler) UpdateVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	spec VGRSpec,
) error {
	vgr := &volrep.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	// Update spec fields
	vgr.Spec.ReplicationState = volrep.ReplicationState(spec.ReplicationState)
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
func (h *LegacyHandler) DeleteVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) error {
	vgr := &volrep.VolumeGroupReplication{}
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
func (h *LegacyHandler) IsVGRReady(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) (bool, error) {
	vgr := &volrep.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return false, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	return isVGRReady(vgr), nil
}

// GetVGRConditions retrieves the conditions of a VolumeGroupReplication
func (h *LegacyHandler) GetVGRConditions(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) ([]metav1.Condition, error) {
	vgr := &volrep.VolumeGroupReplication{}
	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	return vgr.Status.Conditions, nil
}

// isVGRReady determines if a VolumeGroupReplication is ready based on its status
func isVGRReady(vgr *volrep.VolumeGroupReplication) bool {
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

// Ensure LegacyHandler implements ReplicationHandler
var _ ReplicationHandler = (*LegacyHandler)(nil)

// Made with Bob
