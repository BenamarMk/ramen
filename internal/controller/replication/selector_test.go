// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	neutralv1alpha1 "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/replication"
)

// TestSelectHandler_OffloadedTrue tests that when StorageClass has offloaded=true,
// NeutralHandler is selected
func TestSelectHandler_OffloadedTrue(t *testing.T) {
	// Setup: Create a fake client with neutral API CRDs and StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, neutralv1alpha1.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with offloaded label
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "offloaded-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "true",
			},
		},
		Provisioner: "test.csi.driver",
	}

	// Create a neutral VGRC to make neutral API available
	neutralVGRC := &neutralv1alpha1.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-neutral-vgrc",
		},
		Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc, neutralVGRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "offloaded-sc")

	// Assert: Should return NeutralHandler without error
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.NeutralHandlerType, handlerType)
}

// TestSelectHandler_OffloadedFalse tests that when StorageClass has offloaded=false,
// LegacyHandler is selected
func TestSelectHandler_OffloadedFalse(t *testing.T) {
	// Setup: Create a fake client with legacy API CRDs and StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with offloaded=false
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-offloaded-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "false",
			},
		},
		Provisioner: "test.csi.driver",
	}

	// Create a legacy VRC to make legacy API available
	legacyVRC := &volrep.VolumeReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vrc",
		},
		Spec: volrep.VolumeReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc, legacyVRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "non-offloaded-sc")

	// Assert: Should return LegacyHandler without error
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.LegacyHandlerType, handlerType)
}

// TestSelectHandler_NoLabel tests that when StorageClass has no offloaded label,
// LegacyHandler is selected (default behavior)
func TestSelectHandler_NoLabel(t *testing.T) {
	// Setup: Create a fake client with legacy API CRDs and StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass without offloaded label
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-sc",
		},
		Provisioner: "test.csi.driver",
	}

	// Create a legacy VRC to make legacy API available
	legacyVRC := &volrep.VolumeReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vrc",
		},
		Spec: volrep.VolumeReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc, legacyVRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "default-sc")

	// Assert: Should return LegacyHandler without error (default)
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.LegacyHandlerType, handlerType)
}

// TestSelectHandler_StorageClassNotFound tests that when StorageClass doesn't exist,
// an error is returned
func TestSelectHandler_StorageClassNotFound(t *testing.T) {
	// Setup: Create a fake client with no StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, storagev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler for non-existent StorageClass
	handler, handlerType, err := selector.SelectHandler(context.Background(), "non-existent-sc")

	// Assert: Should return error
	require.Error(t, err)
	assert.Nil(t, handler)
	assert.Empty(t, handlerType)
	assert.Contains(t, err.Error(), "failed to get StorageClass")
}

// TestSelectHandler_NeutralAPINotAvailable tests that when offloaded=true but
// neutral API is not available, an error is returned
func TestSelectHandler_NeutralAPINotAvailable(t *testing.T) {
	// Setup: Create a fake client with StorageClass but WITHOUT neutral API scheme
	// This simulates the CRD not being installed
	scheme := runtime.NewScheme()
	// DO NOT add neutralv1alpha1.AddToScheme - this simulates API not available
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with offloaded=true
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "offloaded-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "true",
			},
		},
		Provisioner: "test.csi.driver",
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "offloaded-sc")

	// Assert: Should return error because neutral API is not available
	// IsAvailable returns error when scheme is not registered
	require.Error(t, err)
	assert.Nil(t, handler)
	assert.Empty(t, handlerType)
	assert.Contains(t, err.Error(), "failed to check neutral API availability")
}

// TestSelectHandler_LegacyAPINotAvailable tests that when offloaded=false but
// legacy API is not available, an error is returned
func TestSelectHandler_LegacyAPINotAvailable(t *testing.T) {
	// Setup: Create a fake client with StorageClass but WITHOUT legacy API scheme
	// This simulates the CRD not being installed
	scheme := runtime.NewScheme()
	// DO NOT add volrep.AddToScheme - this simulates API not available
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with offloaded=false
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-offloaded-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "false",
			},
		},
		Provisioner: "test.csi.driver",
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "non-offloaded-sc")

	// Assert: Should return error because legacy API is not available
	// IsAvailable returns error when scheme is not registered
	require.Error(t, err)
	assert.Nil(t, handler)
	assert.Empty(t, handlerType)
	assert.Contains(t, err.Error(), "failed to check legacy API availability")
}

// TestSelectHandlerWithFallback_Success tests that fallback works when
// StorageClass exists and has offloaded label
func TestSelectHandlerWithFallback_Success(t *testing.T) {
	// Setup: Create a fake client with neutral API and StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, neutralv1alpha1.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with offloaded=true
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "offloaded-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "true",
			},
		},
		Provisioner: "test.csi.driver",
	}

	// Create a neutral VGRC to make neutral API available
	neutralVGRC := &neutralv1alpha1.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-neutral-vgrc",
		},
		Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc, neutralVGRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler with fallback
	handler, handlerType, err := selector.SelectHandlerWithFallback(context.Background(), "offloaded-sc")

	// Assert: Should return NeutralHandler without error
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.NeutralHandlerType, handlerType)
}

// TestSelectHandlerWithFallback_FallbackToDiscovery tests that when StorageClass
// doesn't exist, fallback to discovery mechanism works
func TestSelectHandlerWithFallback_FallbackToDiscovery(t *testing.T) {
	// Setup: Create a fake client with neutral API but no StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, neutralv1alpha1.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a neutral VGRC to make neutral API available for discovery
	neutralVGRC := &neutralv1alpha1.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-neutral-vgrc",
		},
		Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(neutralVGRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler with fallback for non-existent StorageClass
	handler, handlerType, err := selector.SelectHandlerWithFallback(context.Background(), "non-existent-sc")

	// Assert: Should fall back to discovery and return NeutralHandler
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.NeutralHandlerType, handlerType)
}

// TestSelectHandler_InvalidLabelValue tests that invalid label values are treated as false
func TestSelectHandler_InvalidLabelValue(t *testing.T) {
	// Setup: Create a fake client with legacy API and StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with invalid label value
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-label-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "invalid",
			},
		},
		Provisioner: "test.csi.driver",
	}

	// Create a legacy VRC to make legacy API available
	legacyVRC := &volrep.VolumeReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vrc",
		},
		Spec: volrep.VolumeReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc, legacyVRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "invalid-label-sc")

	// Assert: Should treat as false and return LegacyHandler
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.LegacyHandlerType, handlerType)
}

// TestSelectHandler_EmptyLabelValue tests that empty label values are treated as false
func TestSelectHandler_EmptyLabelValue(t *testing.T) {
	// Setup: Create a fake client with legacy API and StorageClass
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))

	// Create a StorageClass with empty label value
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "empty-label-sc",
			Labels: map[string]string{
				"ramendr.openshift.io/offloaded": "",
			},
		},
		Provisioner: "test.csi.driver",
	}

	// Create a legacy VRC to make legacy API available
	legacyVRC := &volrep.VolumeReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vrc",
		},
		Spec: volrep.VolumeReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sc, legacyVRC).
		Build()

	// Create selector instance
	selector := replication.NewHandlerSelector(fakeClient, logr.Discard())

	// Test: Select handler
	handler, handlerType, err := selector.SelectHandler(context.Background(), "empty-label-sc")

	// Assert: Should treat as false and return LegacyHandler
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, replication.LegacyHandlerType, handlerType)
}

// Made with Bob
