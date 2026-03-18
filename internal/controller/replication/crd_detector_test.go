// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication_test

import (
	"context"
	"testing"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/replication"
	neutral "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCRDDetector_WithVolrepCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = volrep.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	// Create CRD objects
	vgrCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplications.replication.storage.openshift.io",
		},
	}
	vgrClassCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplicationclasses.replication.storage.openshift.io",
		},
	}
	vgrContentCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplicationcontents.replication.storage.openshift.io",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgrCRD, vgrClassCRD, vgrContentCRD).
		Build()
	detector := replication.NewCRDDetector(client)

	ctx := context.Background()

	// Test volrep CRD detection
	assert.True(t, detector.IsVolumeGroupReplicationAvailable(ctx),
		"Should detect volrep VolumeGroupReplication CRD")
	assert.True(t, detector.IsVolumeGroupReplicationClassAvailable(ctx),
		"Should detect volrep VolumeGroupReplicationClass CRD")
	assert.True(t, detector.IsVolumeGroupReplicationContentAvailable(ctx),
		"Should detect volrep VolumeGroupReplicationContent CRD")

	// Test neutral CRD detection (should be false)
	assert.False(t, detector.IsNeutralVolumeGroupReplicationAvailable(ctx),
		"Should not detect neutral VolumeGroupReplication CRD")
}

func TestCRDDetector_WithNeutralCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = neutral.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	// Create CRD objects
	vgrCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplications.replication.storage.io",
		},
	}
	vgrClassCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplicationclasses.replication.storage.io",
		},
	}
	vgrContentCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplicationcontents.replication.storage.io",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgrCRD, vgrClassCRD, vgrContentCRD).
		Build()
	detector := replication.NewCRDDetector(client)

	ctx := context.Background()

	// Test neutral CRD detection
	assert.True(t, detector.IsNeutralVolumeGroupReplicationAvailable(ctx),
		"Should detect neutral VolumeGroupReplication CRD")
	assert.True(t, detector.IsNeutralVolumeGroupReplicationClassAvailable(ctx),
		"Should detect neutral VolumeGroupReplicationClass CRD")
	assert.True(t, detector.IsNeutralVolumeGroupReplicationContentAvailable(ctx),
		"Should detect neutral VolumeGroupReplicationContent CRD")

	// Test volrep CRD detection (should be false)
	assert.False(t, detector.IsVolumeGroupReplicationAvailable(ctx),
		"Should not detect volrep VolumeGroupReplication CRD")
}

func TestCRDDetector_WithNoCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	detector := replication.NewCRDDetector(client)

	ctx := context.Background()

	// Test that no CRDs are detected
	assert.False(t, detector.IsVolumeGroupReplicationAvailable(ctx),
		"Should not detect volrep VolumeGroupReplication CRD")
	assert.False(t, detector.IsNeutralVolumeGroupReplicationAvailable(ctx),
		"Should not detect neutral VolumeGroupReplication CRD")
}

func TestCRDDetector_Caching(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = volrep.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	// Create CRD object
	vgrCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "volumegroupreplications.replication.storage.openshift.io",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vgrCRD).
		Build()
	detector := replication.NewCRDDetector(client)

	ctx := context.Background()

	// First call should detect and cache
	result1 := detector.IsVolumeGroupReplicationAvailable(ctx)
	assert.True(t, result1, "First call should detect CRD")

	// Second call should use cache (same result)
	result2 := detector.IsVolumeGroupReplicationAvailable(ctx)
	assert.True(t, result2, "Second call should use cached result")
	assert.Equal(t, result1, result2, "Cached result should match first result")
}

// Made with Bob
