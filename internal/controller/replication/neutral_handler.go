// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NeutralAPIGroup is the API group for neutral replication
	NeutralAPIGroup = "replication.storage.io"

	// NeutralAPIVersion is the API version for neutral replication
	NeutralAPIVersion = "v1alpha1"
)

var (
	// neutralVGRGVK is the GroupVersionKind for VolumeGroupReplication
	neutralVGRGVK = schema.GroupVersionKind{
		Group:   NeutralAPIGroup,
		Version: NeutralAPIVersion,
		Kind:    "VolumeGroupReplication",
	}

	// neutralVGRCGVK is the GroupVersionKind for VolumeGroupReplicationClass
	neutralVGRCGVK = schema.GroupVersionKind{
		Group:   NeutralAPIGroup,
		Version: NeutralAPIVersion,
		Kind:    "VolumeGroupReplicationClass",
	}

	// neutralVGRGVR is the GroupVersionResource for VolumeGroupReplication
	neutralVGRGVR = schema.GroupVersionResource{
		Group:    NeutralAPIGroup,
		Version:  NeutralAPIVersion,
		Resource: "volumegroupreplications",
	}

	// neutralVGRCGVR is the GroupVersionResource for VolumeGroupReplicationClass
	neutralVGRCGVR = schema.GroupVersionResource{
		Group:    NeutralAPIGroup,
		Version:  NeutralAPIVersion,
		Resource: "volumegroupreplicationclasses",
	}
)

// NeutralHandler implements ReplicationHandler for the neutral replication.storage.io API
// It uses unstructured types to avoid compile-time dependencies on the neutral API
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
// Uses unstructured to avoid compile-time dependency
func (h *NeutralHandler) IsAvailable(ctx context.Context, c client.Client) (bool, error) {
	// Try to list VolumeGroupReplicationClasses to check if CRD exists
	vgrcList := &unstructured.UnstructuredList{}
	vgrcList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   NeutralAPIGroup,
		Version: NeutralAPIVersion,
		Kind:    "VolumeGroupReplicationClassList",
	})

	if err := c.List(ctx, vgrcList, &client.ListOptions{Limit: 1}); err != nil {
		// Check for "no matches for kind" error (CRD not installed)
		if meta.IsNoMatchError(err) || errors.IsNotFound(err) {
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
	vgrcList := &unstructured.UnstructuredList{}
	vgrcList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   NeutralAPIGroup,
		Version: NeutralAPIVersion,
		Kind:    "VolumeGroupReplicationClassList",
	})

	if err := c.List(ctx, vgrcList); err != nil {
		return nil, fmt.Errorf("failed to list VolumeGroupReplicationClasses: %w", err)
	}

	var classes []VGRClassInfo
	for i := range vgrcList.Items {
		vgrc := &vgrcList.Items[i]

		// Check storage ID label
		labels := vgrc.GetLabels()
		sid := labels[StorageIDLabel]
		if sid != storageID {
			continue
		}

		// Get spec fields
		spec, found, err := unstructured.NestedMap(vgrc.Object, "spec")
		if err != nil || !found {
			continue
		}

		// Check schedule parameter
		if schedule != "" {
			params, found, err := unstructured.NestedStringMap(spec, "parameters")
			if err == nil && found {
				schedParam := params[ReplicationScheduleKey]
				if schedParam != schedule {
					continue
				}
			}
		}

		// Extract provisioner
		provisioner, _, _ := unstructured.NestedString(spec, "provisioner")

		// Extract parameters
		params, _, _ := unstructured.NestedStringMap(spec, "parameters")

		// Extract IDs from labels
		rid := labels[ReplicationIDLabel]
		grid := labels[GroupReplicationIDLabel]

		classes = append(classes, VGRClassInfo{
			Name:               vgrc.GetName(),
			Provisioner:        provisioner,
			Parameters:         params,
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
	vgr := &unstructured.Unstructured{}
	vgr.SetGroupVersionKind(neutralVGRGVK)
	vgr.SetName(namespacedName.Name)
	vgr.SetNamespace(namespacedName.Namespace)

	// Build spec
	vgrSpec := map[string]interface{}{
		"replicationState":                string(spec.ReplicationState),
		"volumeGroupReplicationClassName": spec.VGRClassName,
		"autoResync":                      spec.AutoResync,
	}

	// Add replication handle if provided
	if spec.ReplicationHandle != "" {
		vgrSpec["replicationHandle"] = spec.ReplicationHandle
	}

	// Set source selector if provided
	if spec.PVCSelector != nil {
		vgrSpec["source"] = map[string]interface{}{
			"selector": spec.PVCSelector,
		}
	}

	if err := unstructured.SetNestedMap(vgr.Object, vgrSpec, "spec"); err != nil {
		return fmt.Errorf("failed to set VGR spec: %w", err)
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
	vgr := &unstructured.Unstructured{}
	vgr.SetGroupVersionKind(neutralVGRGVK)

	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	status := &VGRStatus{}

	// Extract status fields
	statusMap, found, err := unstructured.NestedMap(vgr.Object, "status")
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}
	if !found {
		// No status yet
		return status, nil
	}

	// Extract state
	if state, found, _ := unstructured.NestedString(statusMap, "state"); found {
		status.State = State(state)
	}

	// Extract message
	if message, found, _ := unstructured.NestedString(statusMap, "message"); found {
		status.Message = message
	}

	// Extract observedGeneration
	if gen, found, _ := unstructured.NestedInt64(statusMap, "observedGeneration"); found {
		status.ObservedGeneration = gen
	}

	// Extract lastSyncTime
	if syncTime, found, _ := unstructured.NestedString(statusMap, "lastSyncTime"); found {
		t, err := time.Parse(time.RFC3339, syncTime)
		if err == nil {
			metaTime := metav1.NewTime(t)
			status.LastSyncTime = &metaTime
		}
	}

	// Extract lastSyncDuration
	if durationStr, found, _ := unstructured.NestedString(statusMap, "lastSyncDuration"); found {
		duration, err := time.ParseDuration(durationStr)
		if err == nil {
			metaDuration := metav1.Duration{Duration: duration}
			status.LastSyncDuration = &metaDuration
		}
	}

	// Extract lastSyncBytes
	if bytes, found, _ := unstructured.NestedInt64(statusMap, "lastSyncBytes"); found {
		status.LastSyncBytes = &bytes
	}

	// Extract conditions
	if conditions, found, _ := unstructured.NestedSlice(statusMap, "conditions"); found {
		for _, cond := range conditions {
			if condMap, ok := cond.(map[string]interface{}); ok {
				condition := metav1.Condition{}
				if typ, found, _ := unstructured.NestedString(condMap, "type"); found {
					condition.Type = typ
				}
				if condStatus, found, _ := unstructured.NestedString(condMap, "status"); found {
					condition.Status = metav1.ConditionStatus(condStatus)
				}
				if reason, found, _ := unstructured.NestedString(condMap, "reason"); found {
					condition.Reason = reason
				}
				if message, found, _ := unstructured.NestedString(condMap, "message"); found {
					condition.Message = message
				}
				if lastTransitionTime, found, _ := unstructured.NestedString(condMap, "lastTransitionTime"); found {
					t, err := time.Parse(time.RFC3339, lastTransitionTime)
					if err == nil {
						condition.LastTransitionTime = metav1.NewTime(t)
					}
				}
				status.Conditions = append(status.Conditions, condition)
			}
		}
	}

	// Extract PVC list
	if pvcList, found, _ := unstructured.NestedStringSlice(statusMap, "persistentVolumeClaimsRefList"); found {
		// Convert []string to []corev1.LocalObjectReference
		for _, pvcName := range pvcList {
			status.PVCList = append(status.PVCList, corev1.LocalObjectReference{Name: pvcName})
		}
	}

	// Determine if ready
	status.Ready = isNeutralVGRReady(vgr)

	return status, nil
}

// UpdateVGR updates an existing VolumeGroupReplication
func (h *NeutralHandler) UpdateVGR(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	spec VGRSpec,
) error {
	vgr := &unstructured.Unstructured{}
	vgr.SetGroupVersionKind(neutralVGRGVK)

	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	// Update spec fields
	if err := unstructured.SetNestedField(vgr.Object, string(spec.ReplicationState), "spec", "replicationState"); err != nil {
		return fmt.Errorf("failed to set replicationState: %w", err)
	}

	if err := unstructured.SetNestedField(vgr.Object, spec.VGRClassName, "spec", "volumeGroupReplicationClassName"); err != nil {
		return fmt.Errorf("failed to set volumeGroupReplicationClassName: %w", err)
	}

	if err := unstructured.SetNestedField(vgr.Object, spec.AutoResync, "spec", "autoResync"); err != nil {
		return fmt.Errorf("failed to set autoResync: %w", err)
	}

	if spec.PVCSelector != nil {
		if err := unstructured.SetNestedField(vgr.Object, spec.PVCSelector, "spec", "source", "selector"); err != nil {
			return fmt.Errorf("failed to set source selector: %w", err)
		}
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
	vgr := &unstructured.Unstructured{}
	vgr.SetGroupVersionKind(neutralVGRGVK)

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
	vgr := &unstructured.Unstructured{}
	vgr.SetGroupVersionKind(neutralVGRGVK)

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
	vgr := &unstructured.Unstructured{}
	vgr.SetGroupVersionKind(neutralVGRGVK)

	if err := c.Get(ctx, namespacedName, vgr); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplication: %w", err)
	}

	var conditions []metav1.Condition

	// Extract conditions from status
	conditionsSlice, found, err := unstructured.NestedSlice(vgr.Object, "status", "conditions")
	if err != nil || !found {
		return conditions, nil
	}

	for _, cond := range conditionsSlice {
		if condMap, ok := cond.(map[string]interface{}); ok {
			condition := metav1.Condition{}
			if typ, found, _ := unstructured.NestedString(condMap, "type"); found {
				condition.Type = typ
			}
			if condStatus, found, _ := unstructured.NestedString(condMap, "status"); found {
				condition.Status = metav1.ConditionStatus(condStatus)
			}
			if reason, found, _ := unstructured.NestedString(condMap, "reason"); found {
				condition.Reason = reason
			}
			if message, found, _ := unstructured.NestedString(condMap, "message"); found {
				condition.Message = message
			}
			if lastTransitionTime, found, _ := unstructured.NestedString(condMap, "lastTransitionTime"); found {
				t, err := time.Parse(time.RFC3339, lastTransitionTime)
				if err == nil {
					condition.LastTransitionTime = metav1.NewTime(t)
				}
			}
			conditions = append(conditions, condition)
		}
	}

	return conditions, nil
}

// isNeutralVGRReady determines if a VolumeGroupReplication is ready based on its status
func isNeutralVGRReady(vgr *unstructured.Unstructured) bool {
	// Get desired state from spec
	desiredState, found, err := unstructured.NestedString(vgr.Object, "spec", "replicationState")
	if err != nil || !found {
		return false
	}

	// Get current state from status
	currentState, found, err := unstructured.NestedString(vgr.Object, "status", "state")
	if err != nil || !found {
		return false
	}

	// Map states for comparison (case-insensitive)
	if desiredState == "primary" && currentState != "Primary" {
		return false
	}
	if desiredState == "secondary" && currentState != "Secondary" {
		return false
	}

	// Check conditions for any errors
	conditions, found, err := unstructured.NestedSlice(vgr.Object, "status", "conditions")
	if err == nil && found {
		for _, cond := range conditions {
			if condMap, ok := cond.(map[string]interface{}); ok {
				condType, _, _ := unstructured.NestedString(condMap, "type")
				condStatus, _, _ := unstructured.NestedString(condMap, "status")

				if condType == "Degraded" && condStatus == string(metav1.ConditionTrue) {
					return false
				}
				if condType == "Ready" && condStatus == string(metav1.ConditionFalse) {
					return false
				}
			}
		}
	}

	return true
}

// Ensure NeutralHandler implements ReplicationHandler
var _ ReplicationHandler = (*NeutralHandler)(nil)

// Made with Bob
