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
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReplicationFactory_WithVolrepCRDs(t *testing.T) {
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
	ctx := context.Background()

	factory := replication.NewReplicationFactory(ctx, client)

	// Test type selection
	assert.True(t, factory.IsUsingVolrep(), "Should be using volrep")
	assert.False(t, factory.IsUsingNeutral(), "Should not be using neutral")

	// Test object creation
	vgr := factory.NewVolumeGroupReplication("test-vgr", "test-ns")
	require.NotNil(t, vgr, "Should create VGR")
	assert.Equal(t, "test-vgr", vgr.GetName())
	assert.Equal(t, "test-ns", vgr.GetNamespace())

	vgrClass := factory.NewVolumeGroupReplicationClass("test-class")
	require.NotNil(t, vgrClass, "Should create VGRClass")
	assert.Equal(t, "test-class", vgrClass.GetName())

	vgrContent := factory.NewVolumeGroupReplicationContent("test-content")
	require.NotNil(t, vgrContent, "Should create VGRContent")
	assert.Equal(t, "test-content", vgrContent.GetName())
}

func TestReplicationFactory_WithNeutralCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = neutral.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	factory := replication.NewReplicationFactory(ctx, client)

	// Test type selection
	assert.False(t, factory.IsUsingVolrep(), "Should not be using volrep")
	assert.True(t, factory.IsUsingNeutral(), "Should be using neutral")

	// Test object creation
	vgr := factory.NewVolumeGroupReplication("test-vgr", "test-ns")
	require.NotNil(t, vgr, "Should create VGR")
	assert.Equal(t, "test-vgr", vgr.GetName())
	assert.Equal(t, "test-ns", vgr.GetNamespace())
}

func TestReplicationFactory_GetTypes(t *testing.T) {
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
	ctx := context.Background()

	factory := replication.NewReplicationFactory(ctx, client)

	// Test getting type instances
	vgrType := factory.GetVolumeGroupReplicationType()
	require.NotNil(t, vgrType, "Should get VGR type")
	_, ok := vgrType.(*volrep.VolumeGroupReplication)
	assert.True(t, ok, "Should be volrep type")

	vgrClassType := factory.GetVolumeGroupReplicationClassType()
	require.NotNil(t, vgrClassType, "Should get VGRClass type")
	_, ok = vgrClassType.(*volrep.VolumeGroupReplicationClass)
	assert.True(t, ok, "Should be volrep type")

	vgrContentType := factory.GetVolumeGroupReplicationContentType()
	require.NotNil(t, vgrContentType, "Should get VGRContent type")
	_, ok = vgrContentType.(*volrep.VolumeGroupReplicationContent)
	assert.True(t, ok, "Should be volrep type")
}

func TestReplicationFactory_WrapObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = volrep.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	factory := replication.NewReplicationFactory(ctx, client)

	// Create a volrep VGR
	vgr := &volrep.VolumeGroupReplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vgr",
			Namespace: "test-ns",
		},
	}

	// Wrap it
	wrapped := factory.WrapVolumeGroupReplication(vgr)
	require.NotNil(t, wrapped, "Should wrap VGR")
	assert.Equal(t, "test-vgr", wrapped.GetName())
	assert.Equal(t, "test-ns", wrapped.GetNamespace())

	// Verify it's a client.Object (which it inherits from the interface)
	assert.Implements(t, (*replication.VolumeGroupReplicationInterface)(nil), wrapped,
		"Should implement VolumeGroupReplicationInterface")
}

// Made with Bob
