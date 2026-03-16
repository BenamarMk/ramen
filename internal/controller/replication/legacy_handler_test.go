// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication_test

import (
	"context"
	"testing"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/ramendr/ramen/internal/controller/replication"
)

// TestLegacyHandler_GetAPIGroup tests the API group getter
func TestLegacyHandler_GetAPIGroup(t *testing.T) {
	handler := replication.NewLegacyHandler()
	assert.Equal(t, "replication.storage.openshift.io", handler.GetAPIGroup())
}

// TestLegacyHandler_GetAPIVersion tests the API version getter
func TestLegacyHandler_GetAPIVersion(t *testing.T) {
	handler := replication.NewLegacyHandler()
	assert.Equal(t, "v1alpha1", handler.GetAPIVersion())
}

// TestLegacyHandler_IsAvailable tests API availability detection
func TestLegacyHandler_IsAvailable(t *testing.T) {
	tests := []struct {
		name          string
		setupScheme   func(*runtime.Scheme) error
		createObjects func() []runtime.Object
		expected      bool
		expectError   bool
	}{
		{
			name: "API available with objects",
			setupScheme: func(s *runtime.Scheme) error {
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&volrep.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "test-vgrc"},
						Spec: volrep.VolumeGroupReplicationClassSpec{
							Provisioner: "openshift-storage.cephfs.csi.ceph.com",
						},
					},
				}
			},
			expected:    true,
			expectError: false,
		},
		{
			name: "API available without objects",
			setupScheme: func(s *runtime.Scheme) error {
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{}
			},
			expected:    true,
			expectError: false,
		},
		{
			name: "API not available",
			setupScheme: func(s *runtime.Scheme) error {
				return nil
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{}
			},
			expected:    false,
			expectError: true, // When scheme not registered, List returns error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if tt.setupScheme != nil {
				require.NoError(t, tt.setupScheme(scheme))
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.createObjects()...).
				Build()

			handler := replication.NewLegacyHandler()
			available, err := handler.IsAvailable(context.Background(), fakeClient)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, available)
			}
		})
	}
}

// TestLegacyHandler_CreateVGR tests VGR creation with legacy API
func TestLegacyHandler_CreateVGR(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := replication.NewLegacyHandler()

	spec := replication.VGRSpec{
		ReplicationState: replication.Primary,
		VGRClassName:     "test-vgrc",
		PVCSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
		AutoResync:        true,
		ReplicationHandle: "test-handle", // May not be used in legacy API
	}

	namespacedName := types.NamespacedName{
		Name:      "test-vgr",
		Namespace: "test-ns",
	}

	// Test: Create VGR
	err := handler.CreateVGR(context.Background(), fakeClient, namespacedName, spec)
	require.NoError(t, err, "CreateVGR should succeed")

	// Verify: VGR was created
	vgr := &volrep.VolumeGroupReplication{}
	err = fakeClient.Get(context.Background(), namespacedName, vgr)
	require.NoError(t, err, "Should be able to get created VGR")

	// Assert: VGR fields are correct
	assert.Equal(t, "test-vgr", vgr.Name)
	assert.Equal(t, "test-ns", vgr.Namespace)
	assert.Equal(t, volrep.ReplicationState("primary"), vgr.Spec.ReplicationState)
	assert.Equal(t, "test-vgrc", vgr.Spec.VolumeGroupReplicationClassName)
	assert.True(t, vgr.Spec.AutoResync)
	assert.NotNil(t, vgr.Spec.Source.Selector)
	assert.Equal(t, "test", vgr.Spec.Source.Selector.MatchLabels["app"])
}

// TestLegacyHandler_GetVGR tests VGR retrieval with legacy API
func TestLegacyHandler_GetVGR(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	// Create a VGR with status
	vgr := &volrep.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vgr",
			Namespace:  "test-ns",
			Generation: 1,
		},
		Spec: volrep.VolumeGroupReplicationSpec{
			ReplicationState:                volrep.ReplicationState("primary"),
			VolumeGroupReplicationClassName: "test-vgrc",
		},
		Status: volrep.VolumeGroupReplicationStatus{
			VolumeReplicationStatus: volrep.VolumeReplicationStatus{
				State:              "Primary",
				Message:            "Replication is healthy",
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
					},
				},
			},
			PersistentVolumeClaimsRefList: []corev1.LocalObjectReference{
				{Name: "pvc-1"},
				{Name: "pvc-2"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgr).
		Build()

	handler := replication.NewLegacyHandler()

	namespacedName := types.NamespacedName{
		Name:      "test-vgr",
		Namespace: "test-ns",
	}

	// Test: Get VGR
	status, err := handler.GetVGR(context.Background(), fakeClient, namespacedName)
	require.NoError(t, err, "GetVGR should succeed")
	require.NotNil(t, status)

	// Assert: Status fields are correct
	assert.Equal(t, replication.State("Primary"), status.State)
	assert.Equal(t, "Replication is healthy", status.Message)
	assert.Equal(t, int64(1), status.ObservedGeneration)
	assert.Len(t, status.Conditions, 1)
	assert.Equal(t, "Ready", status.Conditions[0].Type)
	assert.Len(t, status.PVCList, 2)
	assert.True(t, status.Ready)
}

// TestLegacyHandler_GetVGR_NotFound tests error handling for non-existent VGR
func TestLegacyHandler_GetVGR_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := replication.NewLegacyHandler()

	namespacedName := types.NamespacedName{
		Name:      "non-existent",
		Namespace: "test-ns",
	}

	// Test: Get non-existent VGR
	status, err := handler.GetVGR(context.Background(), fakeClient, namespacedName)
	require.Error(t, err, "GetVGR should fail for non-existent VGR")
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "failed to get VolumeGroupReplication")
}

// TestLegacyHandler_UpdateVGR tests VGR updates with legacy API
func TestLegacyHandler_UpdateVGR(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	// Create initial VGR
	vgr := &volrep.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vgr",
			Namespace: "test-ns",
		},
		Spec: volrep.VolumeGroupReplicationSpec{
			ReplicationState:                volrep.ReplicationState("primary"),
			VolumeGroupReplicationClassName: "test-vgrc",
			AutoResync:                      false,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgr).
		Build()

	handler := replication.NewLegacyHandler()

	// Update spec
	updatedSpec := replication.VGRSpec{
		ReplicationState: replication.Secondary,
		VGRClassName:     "test-vgrc",
		AutoResync:       true,
		PVCSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"updated": "true",
			},
		},
	}

	namespacedName := types.NamespacedName{
		Name:      "test-vgr",
		Namespace: "test-ns",
	}

	// Test: Update VGR
	err := handler.UpdateVGR(context.Background(), fakeClient, namespacedName, updatedSpec)
	require.NoError(t, err, "UpdateVGR should succeed")

	// Verify: VGR was updated
	updatedVGR := &volrep.VolumeGroupReplication{}
	err = fakeClient.Get(context.Background(), namespacedName, updatedVGR)
	require.NoError(t, err)

	// Assert: Updated fields
	assert.Equal(t, volrep.ReplicationState("secondary"), updatedVGR.Spec.ReplicationState)
	assert.True(t, updatedVGR.Spec.AutoResync)
	assert.Equal(t, "true", updatedVGR.Spec.Source.Selector.MatchLabels["updated"])
}

// TestLegacyHandler_DeleteVGR tests VGR deletion with legacy API
func TestLegacyHandler_DeleteVGR(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	// Create VGR to delete
	vgr := &volrep.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vgr",
			Namespace: "test-ns",
		},
		Spec: volrep.VolumeGroupReplicationSpec{
			ReplicationState:                volrep.ReplicationState("primary"),
			VolumeGroupReplicationClassName: "test-vgrc",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgr).
		Build()

	handler := replication.NewLegacyHandler()

	namespacedName := types.NamespacedName{
		Name:      "test-vgr",
		Namespace: "test-ns",
	}

	// Test: Delete VGR
	err := handler.DeleteVGR(context.Background(), fakeClient, namespacedName)
	require.NoError(t, err, "DeleteVGR should succeed")

	// Verify: VGR was deleted
	deletedVGR := &volrep.VolumeGroupReplication{}
	err = fakeClient.Get(context.Background(), namespacedName, deletedVGR)
	require.Error(t, err, "VGR should not exist after deletion")
}

// TestLegacyHandler_DeleteVGR_AlreadyDeleted tests idempotent deletion
func TestLegacyHandler_DeleteVGR_AlreadyDeleted(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := replication.NewLegacyHandler()

	namespacedName := types.NamespacedName{
		Name:      "non-existent",
		Namespace: "test-ns",
	}

	// Test: Delete non-existent VGR (should succeed - idempotent)
	err := handler.DeleteVGR(context.Background(), fakeClient, namespacedName)
	require.NoError(t, err, "DeleteVGR should be idempotent")
}

// TestLegacyHandler_IsVGRReady tests readiness checking with legacy API
func TestLegacyHandler_IsVGRReady(t *testing.T) {
	tests := []struct {
		name     string
		vgr      *volrep.VolumeGroupReplication
		expected bool
	}{
		{
			name: "Ready - Primary state matches",
			vgr: &volrep.VolumeGroupReplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vgr",
					Namespace: "test-ns",
				},
				Spec: volrep.VolumeGroupReplicationSpec{
					ReplicationState: volrep.ReplicationState("primary"),
				},
				Status: volrep.VolumeGroupReplicationStatus{
					VolumeReplicationStatus: volrep.VolumeReplicationStatus{
						State: "Primary",
						Conditions: []metav1.Condition{
							{Type: "Ready", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Ready - Secondary state matches",
			vgr: &volrep.VolumeGroupReplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vgr",
					Namespace: "test-ns",
				},
				Spec: volrep.VolumeGroupReplicationSpec{
					ReplicationState: volrep.ReplicationState("secondary"),
				},
				Status: volrep.VolumeGroupReplicationStatus{
					VolumeReplicationStatus: volrep.VolumeReplicationStatus{
						State: "Secondary",
						Conditions: []metav1.Condition{
							{Type: "Ready", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Not ready - State mismatch",
			vgr: &volrep.VolumeGroupReplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vgr",
					Namespace: "test-ns",
				},
				Spec: volrep.VolumeGroupReplicationSpec{
					ReplicationState: volrep.ReplicationState("primary"),
				},
				Status: volrep.VolumeGroupReplicationStatus{
					VolumeReplicationStatus: volrep.VolumeReplicationStatus{
						State: "Secondary",
					},
				},
			},
			expected: false,
		},
		{
			name: "Not ready - Degraded condition",
			vgr: &volrep.VolumeGroupReplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vgr",
					Namespace: "test-ns",
				},
				Spec: volrep.VolumeGroupReplicationSpec{
					ReplicationState: volrep.ReplicationState("primary"),
				},
				Status: volrep.VolumeGroupReplicationStatus{
					VolumeReplicationStatus: volrep.VolumeReplicationStatus{
						State: "Primary",
						Conditions: []metav1.Condition{
							{Type: "Degraded", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Not ready - Ready condition false",
			vgr: &volrep.VolumeGroupReplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vgr",
					Namespace: "test-ns",
				},
				Spec: volrep.VolumeGroupReplicationSpec{
					ReplicationState: volrep.ReplicationState("primary"),
				},
				Status: volrep.VolumeGroupReplicationStatus{
					VolumeReplicationStatus: volrep.VolumeReplicationStatus{
						State: "Primary",
						Conditions: []metav1.Condition{
							{Type: "Ready", Status: metav1.ConditionFalse},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, volrep.AddToScheme(scheme))

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.vgr).
				Build()

			handler := replication.NewLegacyHandler()

			namespacedName := types.NamespacedName{
				Name:      tt.vgr.Name,
				Namespace: tt.vgr.Namespace,
			}

			ready, err := handler.IsVGRReady(context.Background(), fakeClient, namespacedName)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ready)
		})
	}
}

// TestLegacyHandler_GetVGRConditions tests condition retrieval with legacy API
func TestLegacyHandler_GetVGRConditions(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	vgr := &volrep.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vgr",
			Namespace: "test-ns",
		},
		Status: volrep.VolumeGroupReplicationStatus{
			VolumeReplicationStatus: volrep.VolumeReplicationStatus{
				Conditions: []metav1.Condition{
					{
						Type:    "Ready",
						Status:  metav1.ConditionTrue,
						Reason:  "ReplicationHealthy",
						Message: "Replication is working",
					},
					{
						Type:    "Syncing",
						Status:  metav1.ConditionFalse,
						Reason:  "SyncComplete",
						Message: "Last sync completed successfully",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgr).
		Build()

	handler := replication.NewLegacyHandler()

	namespacedName := types.NamespacedName{
		Name:      "test-vgr",
		Namespace: "test-ns",
	}

	// Test: Get conditions
	conditions, err := handler.GetVGRConditions(context.Background(), fakeClient, namespacedName)
	require.NoError(t, err)
	require.Len(t, conditions, 2)

	// Assert: Conditions are correct
	assert.Equal(t, "Ready", conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
	assert.Equal(t, "Syncing", conditions[1].Type)
	assert.Equal(t, metav1.ConditionFalse, conditions[1].Status)
}

// TestLegacyHandler_DiscoverVGRClasses tests VGRClass discovery with legacy API
func TestLegacyHandler_DiscoverVGRClasses(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	// Create VGRClasses with different labels and parameters
	vgrc1 := &volrep.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vgrc-storage1-5m",
			Labels: map[string]string{
				"ramendr.openshift.io/storageid":            "storage1",
				"ramendr.openshift.io/replicationid":        "rep1",
				"ramendr.openshift.io/groupreplicationid":   "group1",
			},
		},
		Spec: volrep.VolumeGroupReplicationClassSpec{
			Provisioner: "openshift-storage.cephfs.csi.ceph.com",
			Parameters: map[string]string{
				"schedulingInterval": "5m",
			},
		},
	}

	vgrc2 := &volrep.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vgrc-storage1-10m",
			Labels: map[string]string{
				"ramendr.openshift.io/storageid":            "storage1",
				"ramendr.openshift.io/replicationid":        "rep2",
				"ramendr.openshift.io/groupreplicationid":   "group2",
			},
		},
		Spec: volrep.VolumeGroupReplicationClassSpec{
			Provisioner: "openshift-storage.cephfs.csi.ceph.com",
			Parameters: map[string]string{
				"schedulingInterval": "10m",
			},
		},
	}

	vgrc3 := &volrep.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vgrc-storage2-5m",
			Labels: map[string]string{
				"ramendr.openshift.io/storageid": "storage2",
			},
		},
		Spec: volrep.VolumeGroupReplicationClassSpec{
			Provisioner: "openshift-storage.cephfs.csi.ceph.com",
			Parameters: map[string]string{
				"schedulingInterval": "5m",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgrc1, vgrc2, vgrc3).
		Build()

	handler := replication.NewLegacyHandler()

	tests := []struct {
		name             string
		storageClassName string
		storageID        string
		schedule         string
		expectedCount    int
		expectedNames    []string
	}{
		{
			name:             "Find all for storage1",
			storageClassName: "",
			storageID:        "storage1",
			schedule:         "",
			expectedCount:    2,
			expectedNames:    []string{"vgrc-storage1-5m", "vgrc-storage1-10m"},
		},
		{
			name:             "Find storage1 with 5m schedule",
			storageClassName: "",
			storageID:        "storage1",
			schedule:         "5m",
			expectedCount:    1,
			expectedNames:    []string{"vgrc-storage1-5m"},
		},
		{
			name:             "Find storage2",
			storageClassName: "",
			storageID:        "storage2",
			schedule:         "",
			expectedCount:    1,
			expectedNames:    []string{"vgrc-storage2-5m"},
		},
		{
			name:             "No match for non-existent storage",
			storageClassName: "",
			storageID:        "storage3",
			schedule:         "",
			expectedCount:    0,
			expectedNames:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classes, err := handler.DiscoverVGRClasses(
				context.Background(),
				fakeClient,
				tt.storageClassName,
				tt.storageID,
				tt.schedule,
			)
			require.NoError(t, err)
			assert.Len(t, classes, tt.expectedCount)

			// Verify class names
			classNames := make([]string, len(classes))
			for i, class := range classes {
				classNames[i] = class.Name
			}
			for _, expectedName := range tt.expectedNames {
				assert.Contains(t, classNames, expectedName)
			}
		})
	}
}

// TestLegacyHandler_BackwardCompatibility tests that legacy handler maintains ODF compatibility
func TestLegacyHandler_BackwardCompatibility(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	handler := replication.NewLegacyHandler()

	// Test 1: API group matches ODF expectations
	assert.Equal(t, "replication.storage.openshift.io", handler.GetAPIGroup(),
		"API group must match ODF expectations for backward compatibility")

	// Test 2: Create VGR with typical ODF configuration
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	odfSpec := replication.VGRSpec{
		ReplicationState: replication.Primary,
		VGRClassName:     "odf-vgrc",
		PVCSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"appname": "busybox",
			},
		},
		AutoResync: true,
	}

	namespacedName := types.NamespacedName{
		Name:      "busybox-vgr",
		Namespace: "busybox-sample",
	}

	err := handler.CreateVGR(context.Background(), fakeClient, namespacedName, odfSpec)
	require.NoError(t, err, "Should create VGR with ODF-style configuration")

	// Test 3: Verify created VGR matches ODF structure
	vgr := &volrep.VolumeGroupReplication{}
	err = fakeClient.Get(context.Background(), namespacedName, vgr)
	require.NoError(t, err)

	assert.Equal(t, "busybox-vgr", vgr.Name)
	assert.Equal(t, "busybox-sample", vgr.Namespace)
	assert.Equal(t, volrep.ReplicationState("primary"), vgr.Spec.ReplicationState)
	assert.Equal(t, "odf-vgrc", vgr.Spec.VolumeGroupReplicationClassName)
	assert.True(t, vgr.Spec.AutoResync)
	assert.Equal(t, "busybox", vgr.Spec.Source.Selector.MatchLabels["appname"])
}

// Made with Bob
