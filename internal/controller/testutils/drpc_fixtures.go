// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	spokeClusterV1 "open-cluster-management.io/api/cluster/v1"
	clrapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	argocdv1alpha1hack "github.com/ramendr/ramen/internal/controller/argocd"
	corev1 "k8s.io/api/core/v1"
)

// Test constants for cluster and resource names
const (
	DRPCCommonName        = "drpc-name"
	DefaultDRPCNamespace  = "drpc-namespace"
	ApplicationNamespace  = "vrg-namespace"
	DRPC2Name             = "drpc-name2"
	DRPC2NamespaceName    = "drpc-namespace2"
	UserPlacementRuleName = "user-placement-rule"
	UserPlacementName     = "user-placement"
	East1ManagedCluster   = "east1-cluster"
	East2ManagedCluster   = "east2-cluster"
	West1ManagedCluster   = "west1-cluster"
	AsyncDRPolicyName     = "my-async-dr-peers"
	SyncDRPolicyName      = "my-sync-dr-peers"
	MModeReplicationID    = "storage-replication-id-1"
	MModeCSIProvisioner   = "test.csi.com"
)

// ClusterBuilder provides a fluent API for building test clusters
type ClusterBuilder struct {
	cluster *spokeClusterV1.ManagedCluster
}

// NewClusterBuilder creates a new ClusterBuilder
func NewClusterBuilder(name string) *ClusterBuilder {
	return &ClusterBuilder{
		cluster: &spokeClusterV1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: make(map[string]string),
			},
		},
	}
}

// WithLabel adds a label to the cluster
func (b *ClusterBuilder) WithLabel(key, value string) *ClusterBuilder {
	b.cluster.Labels[key] = value
	return b
}

// Build returns the constructed cluster
func (b *ClusterBuilder) Build() *spokeClusterV1.ManagedCluster {
	return b.cluster
}

// GetWest1Cluster returns the west1 test cluster
func GetWest1Cluster() *spokeClusterV1.ManagedCluster {
	return NewClusterBuilder(West1ManagedCluster).
		WithLabel("name", West1ManagedCluster).
		WithLabel("key1", "west1").
		Build()
}

// GetEast1Cluster returns the east1 test cluster
func GetEast1Cluster() *spokeClusterV1.ManagedCluster {
	return NewClusterBuilder(East1ManagedCluster).
		WithLabel("name", East1ManagedCluster).
		WithLabel("key1", "east1").
		Build()
}

// GetEast2Cluster returns the east2 test cluster
func GetEast2Cluster() *spokeClusterV1.ManagedCluster {
	return NewClusterBuilder(East2ManagedCluster).
		WithLabel("name", East2ManagedCluster).
		WithLabel("key1", "east2").
		Build()
}

// GetAsyncClusters returns clusters for async DR testing
func GetAsyncClusters() []*spokeClusterV1.ManagedCluster {
	return []*spokeClusterV1.ManagedCluster{GetWest1Cluster(), GetEast1Cluster()}
}

// GetSyncClusters returns clusters for sync DR testing
func GetSyncClusters() []*spokeClusterV1.ManagedCluster {
	return []*spokeClusterV1.ManagedCluster{GetEast1Cluster(), GetEast2Cluster()}
}

// NamespaceBuilder provides a fluent API for building test namespaces
type NamespaceBuilder struct {
	namespace *corev1.Namespace
}

// NewNamespaceBuilder creates a new NamespaceBuilder
func NewNamespaceBuilder(name string) *NamespaceBuilder {
	return &NamespaceBuilder{
		namespace: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		},
	}
}

// Build returns the constructed namespace
func (b *NamespaceBuilder) Build() *corev1.Namespace {
	return b.namespace
}

// GetTestNamespaces returns all test namespaces
func GetTestNamespaces() []*corev1.Namespace {
	return []*corev1.Namespace{
		NewNamespaceBuilder(East1ManagedCluster).Build(),
		NewNamespaceBuilder(West1ManagedCluster).Build(),
		NewNamespaceBuilder(East2ManagedCluster).Build(),
		NewNamespaceBuilder(DefaultDRPCNamespace).Build(),
		NewNamespaceBuilder(DRPC2NamespaceName).Build(),
	}
}

// DRPolicyBuilder provides a fluent API for building DRPolicy objects
type DRPolicyBuilder struct {
	policy *rmn.DRPolicy
}

// NewDRPolicyBuilder creates a new DRPolicyBuilder
func NewDRPolicyBuilder(name string) *DRPolicyBuilder {
	return &DRPolicyBuilder{
		policy: &rmn.DRPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: rmn.DRPolicySpec{},
		},
	}
}

// WithClusters sets the DR clusters
func (b *DRPolicyBuilder) WithClusters(clusters ...string) *DRPolicyBuilder {
	b.policy.Spec.DRClusters = clusters
	return b
}

// WithSchedulingInterval sets the scheduling interval
func (b *DRPolicyBuilder) WithSchedulingInterval(interval string) *DRPolicyBuilder {
	b.policy.Spec.SchedulingInterval = interval
	return b
}

// Build returns the constructed DRPolicy
func (b *DRPolicyBuilder) Build() *rmn.DRPolicy {
	return b.policy
}

// GetAsyncDRPolicy returns a default async DR policy
func GetAsyncDRPolicy() *rmn.DRPolicy {
	return NewDRPolicyBuilder(AsyncDRPolicyName).
		WithClusters(East1ManagedCluster, West1ManagedCluster).
		WithSchedulingInterval("1h").
		Build()
}

// GetSyncDRPolicy returns a default sync DR policy
func GetSyncDRPolicy() *rmn.DRPolicy {
	return NewDRPolicyBuilder(SyncDRPolicyName).
		WithClusters(East1ManagedCluster, East2ManagedCluster).
		Build()
}

// DRPCBuilder provides a fluent API for building DRPlacementControl objects
type DRPCBuilder struct {
	drpc *rmn.DRPlacementControl
}

// NewDRPCBuilder creates a new DRPCBuilder
func NewDRPCBuilder(name, namespace string) *DRPCBuilder {
	return &DRPCBuilder{
		drpc: &rmn.DRPlacementControl{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: rmn.DRPlacementControlSpec{},
		},
	}
}

// WithPlacementRef sets the placement reference
func (b *DRPCBuilder) WithPlacementRef(name, kind string) *DRPCBuilder {
	b.drpc.Spec.PlacementRef = corev1.ObjectReference{
		Name: name,
		Kind: kind,
	}
	return b
}

// WithDRPolicyRef sets the DR policy reference
func (b *DRPCBuilder) WithDRPolicyRef(name string) *DRPCBuilder {
	b.drpc.Spec.DRPolicyRef = corev1.ObjectReference{
		Name: name,
	}
	return b
}

// WithPreferredCluster sets the preferred cluster
func (b *DRPCBuilder) WithPreferredCluster(cluster string) *DRPCBuilder {
	b.drpc.Spec.PreferredCluster = cluster
	return b
}

// WithFailoverCluster sets the failover cluster
func (b *DRPCBuilder) WithFailoverCluster(cluster string) *DRPCBuilder {
	b.drpc.Spec.FailoverCluster = cluster
	return b
}

// WithAction sets the DR action
func (b *DRPCBuilder) WithAction(action rmn.DRAction) *DRPCBuilder {
	b.drpc.Spec.Action = action
	return b
}

// WithPVCSelector sets the PVC selector
func (b *DRPCBuilder) WithPVCSelector(selector metav1.LabelSelector) *DRPCBuilder {
	b.drpc.Spec.PVCSelector = selector
	return b
}

// Build returns the constructed DRPlacementControl
func (b *DRPCBuilder) Build() *rmn.DRPlacementControl {
	return b.drpc
}

// VRGBuilder provides a fluent API for building VolumeReplicationGroup objects
type VRGBuilder struct {
	vrg *rmn.VolumeReplicationGroup
}

// NewVRGBuilder creates a new VRGBuilder
func NewVRGBuilder(name, namespace string) *VRGBuilder {
	return &VRGBuilder{
		vrg: &rmn.VolumeReplicationGroup{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VolumeReplicationGroup",
				APIVersion: "ramendr.openshift.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: rmn.VolumeReplicationGroupSpec{},
		},
	}
}

// WithReplicationState sets the replication state
func (b *VRGBuilder) WithReplicationState(state rmn.ReplicationState) *VRGBuilder {
	b.vrg.Spec.ReplicationState = state
	return b
}

// WithAsync sets async configuration
func (b *VRGBuilder) WithAsync(interval string) *VRGBuilder {
	b.vrg.Spec.Async = &rmn.VRGAsyncSpec{
		SchedulingInterval: interval,
	}
	return b
}

// WithPVCSelector sets the PVC selector
func (b *VRGBuilder) WithPVCSelector(selector metav1.LabelSelector) *VRGBuilder {
	b.vrg.Spec.PVCSelector = selector
	return b
}

// WithS3Profiles sets the S3 profiles
func (b *VRGBuilder) WithS3Profiles(profiles ...string) *VRGBuilder {
	b.vrg.Spec.S3Profiles = profiles
	return b
}

// Build returns the constructed VolumeReplicationGroup
func (b *VRGBuilder) Build() *rmn.VolumeReplicationGroup {
	return b.vrg
}

// GetDefaultVRG returns a default VRG for testing
func GetDefaultVRG(namespace string, s3ProfileName string) *rmn.VolumeReplicationGroup {
	return NewVRGBuilder(DRPCCommonName, namespace).
		WithReplicationState(rmn.Primary).
		WithAsync("1h").
		WithPVCSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"appclass": "gold"},
		}).
		WithS3Profiles(s3ProfileName).
		Build()
}

// ApplicationSetBuilder provides a fluent API for building ApplicationSet objects
type ApplicationSetBuilder struct {
	appSet *argocdv1alpha1hack.ApplicationSet
}

// NewApplicationSetBuilder creates a new ApplicationSetBuilder
func NewApplicationSetBuilder(name, namespace string) *ApplicationSetBuilder {
	return &ApplicationSetBuilder{
		appSet: &argocdv1alpha1hack.ApplicationSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: argocdv1alpha1hack.ApplicationSetSpec{},
		},
	}
}

// WithPlacementRef sets the placement reference in generators
func (b *ApplicationSetBuilder) WithPlacementRef(placementName string) *ApplicationSetBuilder {
	b.appSet.Spec.Generators = []argocdv1alpha1hack.ApplicationSetGenerator{
		{
			ClusterDecisionResource: &argocdv1alpha1hack.DuckTypeGenerator{
				LabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						clrapiv1beta1.PlacementLabel: placementName,
					},
				},
			},
		},
	}
	return b
}

// WithDestinationNamespace sets the destination namespace
func (b *ApplicationSetBuilder) WithDestinationNamespace(namespace string) *ApplicationSetBuilder {
	b.appSet.Spec.Template = argocdv1alpha1hack.ApplicationSetTemplate{
		Spec: argocdv1alpha1hack.ApplicationSpec{
			Project: "default",
			Destination: argocdv1alpha1hack.ApplicationDestination{
				Namespace: namespace,
			},
		},
	}
	return b
}

// Build returns the constructed ApplicationSet
func (b *ApplicationSetBuilder) Build() *argocdv1alpha1hack.ApplicationSet {
	return b.appSet
}

// GetDefaultApplicationSet returns a default ApplicationSet for testing
func GetDefaultApplicationSet() *argocdv1alpha1hack.ApplicationSet {
	return NewApplicationSetBuilder("simple-appset", DefaultDRPCNamespace).
		WithPlacementRef(UserPlacementName).
		WithDestinationNamespace(ApplicationNamespace).
		Build()
}

// GetTestCIDRs returns test CIDR ranges for cluster fencing
func GetTestCIDRs() [][]string {
	return [][]string{
		{"198.51.100.17/24", "198.51.100.18/24", "198.51.100.19/24"},
		{"198.51.100.20/24", "198.51.100.21/24", "198.51.100.22/24"},
	}
}

// Made with Bob
