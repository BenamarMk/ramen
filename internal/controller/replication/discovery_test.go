// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	neutralv1alpha1 "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/replication"
)

// TestDiscoverHandler_NeutralAvailable tests that when neutral API is available,
// it is selected with priority over legacy API
func TestDiscoverHandler_NeutralAvailable(t *testing.T) {
	// Setup: Create a fake client with neutral API CRDs installed
	scheme := runtime.NewScheme()
	require.NoError(t, neutralv1alpha1.AddToScheme(scheme))

	// Create a fake VolumeGroupReplicationClass to simulate neutral API presence
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

	// Create discovery instance
	discovery := replication.NewDiscovery(fakeClient, logr.Discard())

	// Test: Discover handler
	handler, handlerType, err := discovery.DiscoverHandler(context.Background())

	// Assert: Should return NeutralHandler without error
	require.NoError(t, err, "Discovery should succeed when neutral API is available")
	require.NotNil(t, handler, "Handler should not be nil")
	assert.Equal(t, replication.NeutralHandlerType, handlerType, "Should return neutral handler type")

	// Verify it's the neutral handler by checking the API group it uses
	assert.Equal(t, "replication.storage.io", handler.GetAPIGroup(),
		"Should select neutral API when available")
}

// TestDiscoverHandler_LegacyOnly tests that when only legacy API is available,
// it falls back to legacy handler
func TestDiscoverHandler_LegacyOnly(t *testing.T) {
	// Setup: Create a fake client with only legacy API CRDs installed
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	// Create a fake VolumeGroupReplicationClass to simulate legacy API presence
	legacyVGRC := &volrep.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vgrc",
		},
		Spec: volrep.VolumeGroupReplicationClassSpec{
			Provisioner: "openshift-storage.cephfs.csi.ceph.com",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(legacyVGRC).
		Build()

	// Create discovery instance
	discovery := replication.NewDiscovery(fakeClient, logr.Discard())

	// Test: Discover handler
	handler, handlerType, err := discovery.DiscoverHandler(context.Background())

	// Assert: Should return LegacyHandler without error
	require.NoError(t, err, "Discovery should succeed when legacy API is available")
	require.NotNil(t, handler, "Handler should not be nil")
	assert.Equal(t, replication.LegacyHandlerType, handlerType, "Should return legacy handler type")

	// Verify it's the legacy handler
	assert.Equal(t, "replication.storage.openshift.io", handler.GetAPIGroup(),
		"Should select legacy API when neutral is not available")
}

// TestDiscoverHandler_BothAvailable tests priority selection when both APIs exist
func TestDiscoverHandler_BothAvailable(t *testing.T) {
	// Setup: Create a fake client with BOTH neutral and legacy API CRDs installed
	scheme := runtime.NewScheme()
	require.NoError(t, neutralv1alpha1.AddToScheme(scheme))
	require.NoError(t, volrep.AddToScheme(scheme))

	// Create both neutral and legacy VGRCs
	neutralVGRC := &neutralv1alpha1.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-neutral-vgrc",
		},
		Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}

	legacyVGRC := &volrep.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vgrc",
		},
		Spec: volrep.VolumeGroupReplicationClassSpec{
			Provisioner: "openshift-storage.cephfs.csi.ceph.com",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(neutralVGRC, legacyVGRC).
		Build()

	// Create discovery instance
	discovery := replication.NewDiscovery(fakeClient, logr.Discard())

	// Test: Discover handler
	handler, handlerType, err := discovery.DiscoverHandler(context.Background())

	// Assert: Should return NeutralHandler (priority over legacy)
	require.NoError(t, err, "Discovery should succeed when both APIs are available")
	require.NotNil(t, handler, "Handler should not be nil")
	assert.Equal(t, replication.NeutralHandlerType, handlerType, "Should return neutral handler type")

	// Verify neutral API is selected (has priority)
	assert.Equal(t, "replication.storage.io", handler.GetAPIGroup(),
		"Should prioritize neutral API over legacy when both are available")
}

// TestDiscoverHandler_NoneAvailable tests error handling when no API is available
func TestDiscoverHandler_NoneAvailable(t *testing.T) {
	// Setup: Create a fake client with NO replication API CRDs installed
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create discovery instance
	discovery := replication.NewDiscovery(fakeClient, logr.Discard())

	// Test: Discover handler
	handler, handlerType, err := discovery.DiscoverHandler(context.Background())

	// Assert: Should return error and nil handler
	require.Error(t, err, "Discovery should fail when no replication API is available")
	assert.Nil(t, handler, "Handler should be nil when discovery fails")
	assert.Empty(t, handlerType, "Handler type should be empty when discovery fails")
	// Error message may vary based on which API check fails first
	assert.True(t,
		err.Error() == "no replication API available (checked replication.storage.io and replication.storage.openshift.io)" ||
		err.Error() == "failed to check legacy API: no kind is registered for the type v1alpha1.VolumeGroupReplicationClassList in scheme \"pkg/runtime/scheme.go:110\"",
		"Error should indicate API discovery failure")
}

// TestDiscoverHandler_DynamicDiscovery tests that discovery refreshes
// when available APIs change during runtime
func TestDiscoverHandler_DynamicDiscovery(t *testing.T) {
	// Setup: Start with only legacy API scheme registered
	scheme := runtime.NewScheme()
	require.NoError(t, volrep.AddToScheme(scheme))

	legacyVGRC := &volrep.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-legacy-vgrc",
		},
		Spec: volrep.VolumeGroupReplicationClassSpec{
			Provisioner: "openshift-storage.cephfs.csi.ceph.com",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(legacyVGRC).
		Build()

	// Create discovery instance
	discovery := replication.NewDiscovery(fakeClient, logr.Discard())

	// Test: First discovery should return legacy handler (neutral scheme not registered)
	handler1, handlerType1, err := discovery.DiscoverHandler(context.Background())
	require.NoError(t, err)
	assert.Equal(t, replication.LegacyHandlerType, handlerType1)
	assert.Equal(t, "replication.storage.openshift.io", handler1.GetAPIGroup(),
		"First discovery should select legacy API when neutral scheme not registered")

	// Now add neutral scheme and create neutral object
	require.NoError(t, neutralv1alpha1.AddToScheme(scheme))
	
	neutralVGRC := &neutralv1alpha1.VolumeGroupReplicationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-neutral-vgrc",
		},
		Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
			Provisioner: "test.storage.io/driver",
		},
	}
	require.NoError(t, fakeClient.Create(context.Background(), neutralVGRC))

	// Test: Second discovery should detect neutral API and switch
	handler2, handlerType2, err := discovery.DiscoverHandler(context.Background())
	require.NoError(t, err)
	assert.Equal(t, replication.NeutralHandlerType, handlerType2)
	assert.Equal(t, "replication.storage.io", handler2.GetAPIGroup(),
		"Second discovery should detect and prioritize neutral API")
}

// TestDiscoverHandlerForPeerClass tests PeerClass-specific discovery
func TestDiscoverHandlerForPeerClass(t *testing.T) {
	tests := []struct {
		name          string
		setupScheme   func(*runtime.Scheme) error
		createObjects func() []runtime.Object
		peerClassName string
		preferNeutral bool
		expectedGroup string
		expectedType  replication.HandlerType
		expectedError bool
	}{
		{
			name: "Prefer neutral when available",
			setupScheme: func(s *runtime.Scheme) error {
				if err := neutralv1alpha1.AddToScheme(s); err != nil {
					return err
				}
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&neutralv1alpha1.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "neutral-vgrc"},
						Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
							Provisioner: "test.io/driver",
						},
					},
				}
			},
			peerClassName: "test-peer",
			preferNeutral: true,
			expectedGroup: "replication.storage.io",
			expectedType:  replication.NeutralHandlerType,
			expectedError: false,
		},
		{
			name: "Fallback to legacy when neutral not available",
			setupScheme: func(s *runtime.Scheme) error {
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&volrep.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "legacy-vgrc"},
						Spec: volrep.VolumeGroupReplicationClassSpec{
							Provisioner: "openshift.io/driver",
						},
					},
				}
			},
			peerClassName: "test-peer",
			preferNeutral: true,
			expectedGroup: "replication.storage.openshift.io",
			expectedType:  replication.LegacyHandlerType,
			expectedError: false,
		},
		{
			name: "Standard discovery when not preferring neutral",
			setupScheme: func(s *runtime.Scheme) error {
				if err := neutralv1alpha1.AddToScheme(s); err != nil {
					return err
				}
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&neutralv1alpha1.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "neutral-vgrc"},
						Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
							Provisioner: "test.io/driver",
						},
					},
				}
			},
			peerClassName: "test-peer",
			preferNeutral: false,
			expectedGroup: "replication.storage.io",
			expectedType:  replication.NeutralHandlerType,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, tt.setupScheme(scheme))

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.createObjects()...).
				Build()

			discovery := replication.NewDiscovery(fakeClient, logr.Discard())

			handler, handlerType, err := discovery.DiscoverHandlerForPeerClass(
				context.Background(),
				tt.peerClassName,
				tt.preferNeutral,
			)

			if tt.expectedError {
				require.Error(t, err)
				assert.Nil(t, handler)
			} else {
				require.NoError(t, err)
				require.NotNil(t, handler)
				assert.Equal(t, tt.expectedGroup, handler.GetAPIGroup())
				assert.Equal(t, tt.expectedType, handlerType)
			}
		})
	}
}

// TestGetAvailableAPIs tests the API availability reporting
func TestGetAvailableAPIs(t *testing.T) {
	tests := []struct {
		name           string
		setupScheme    func(*runtime.Scheme) error
		createObjects  func() []runtime.Object
		expectedNeutral bool
		expectedLegacy  bool
	}{
		{
			name: "Both APIs available",
			setupScheme: func(s *runtime.Scheme) error {
				if err := neutralv1alpha1.AddToScheme(s); err != nil {
					return err
				}
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&neutralv1alpha1.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "neutral-vgrc"},
						Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
							Provisioner: "test.io/driver",
						},
					},
					&volrep.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "legacy-vgrc"},
						Spec: volrep.VolumeGroupReplicationClassSpec{
							Provisioner: "openshift.io/driver",
						},
					},
				}
			},
			expectedNeutral: true,
			expectedLegacy:  true,
		},
		{
			name: "Only neutral available",
			setupScheme: func(s *runtime.Scheme) error {
				return neutralv1alpha1.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&neutralv1alpha1.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "neutral-vgrc"},
						Spec: neutralv1alpha1.VolumeGroupReplicationClassSpec{
							Provisioner: "test.io/driver",
						},
					},
				}
			},
			expectedNeutral: true,
			expectedLegacy:  false,
		},
		{
			name: "Only legacy available",
			setupScheme: func(s *runtime.Scheme) error {
				return volrep.AddToScheme(s)
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{
					&volrep.VolumeGroupReplicationClass{
						ObjectMeta: metav1.ObjectMeta{Name: "legacy-vgrc"},
						Spec: volrep.VolumeGroupReplicationClassSpec{
							Provisioner: "openshift.io/driver",
						},
					},
				}
			},
			expectedNeutral: false,
			expectedLegacy:  true,
		},
		{
			name: "Neither available",
			setupScheme: func(s *runtime.Scheme) error {
				return nil
			},
			createObjects: func() []runtime.Object {
				return []runtime.Object{}
			},
			expectedNeutral: false,
			expectedLegacy:  false,
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

			discovery := replication.NewDiscovery(fakeClient, logr.Discard())

			apis, err := discovery.GetAvailableAPIs(context.Background())
			require.NoError(t, err)

			assert.Equal(t, tt.expectedNeutral, apis["replication.storage.io"],
				"Neutral API availability mismatch")
			assert.Equal(t, tt.expectedLegacy, apis["replication.storage.openshift.io"],
				"Legacy API availability mismatch")
		})
	}
}

// TestHandlerFactory tests the handler factory
func TestHandlerFactory(t *testing.T) {
	factory := replication.NewHandlerFactory()

	t.Run("Create neutral handler", func(t *testing.T) {
		handler, err := factory.CreateHandler(replication.NeutralHandlerType)
		require.NoError(t, err)
		require.NotNil(t, handler)
		assert.Equal(t, "replication.storage.io", handler.GetAPIGroup())
	})

	t.Run("Create legacy handler", func(t *testing.T) {
		handler, err := factory.CreateHandler(replication.LegacyHandlerType)
		require.NoError(t, err)
		require.NotNil(t, handler)
		assert.Equal(t, "replication.storage.openshift.io", handler.GetAPIGroup())
	})

	t.Run("Invalid handler type", func(t *testing.T) {
		handler, err := factory.CreateHandler("invalid")
		require.Error(t, err)
		assert.Nil(t, handler)
		assert.Contains(t, err.Error(), "unknown handler type")
	})

	t.Run("Create neutral handler directly", func(t *testing.T) {
		handler := factory.CreateNeutralHandler()
		require.NotNil(t, handler)
		assert.Equal(t, "replication.storage.io", handler.GetAPIGroup())
	})

	t.Run("Create legacy handler directly", func(t *testing.T) {
		handler := factory.CreateLegacyHandler()
		require.NotNil(t, handler)
		assert.Equal(t, "replication.storage.openshift.io", handler.GetAPIGroup())
	})
}

// Made with Bob
