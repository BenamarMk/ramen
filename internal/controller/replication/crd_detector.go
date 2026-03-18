// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CRDDetector checks for the availability of CRDs in the cluster
type CRDDetector struct {
	client client.Client
	cache  map[string]bool
	mu     sync.RWMutex
}

// NewCRDDetector creates a new CRD detector
func NewCRDDetector(client client.Client) *CRDDetector {
	return &CRDDetector{
		client: client,
		cache:  make(map[string]bool),
	}
}

// IsCRDAvailable checks if a CRD exists in the cluster
func (d *CRDDetector) IsCRDAvailable(ctx context.Context, crdName string) bool {
	d.mu.RLock()
	if available, exists := d.cache[crdName]; exists {
		d.mu.RUnlock()
		return available
	}
	d.mu.RUnlock()

	// Check if CRD exists
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := d.client.Get(ctx, types.NamespacedName{Name: crdName}, crd)
	available := err == nil

	// Cache the result
	d.mu.Lock()
	d.cache[crdName] = available
	d.mu.Unlock()

	return available
}

// IsVolumeGroupReplicationAvailable checks if VolumeGroupReplication CRD from csi-addons is available
func (d *CRDDetector) IsVolumeGroupReplicationAvailable(ctx context.Context) bool {
	return d.IsCRDAvailable(ctx, "volumegroupreplications.replication.storage.openshift.io")
}

// IsVolumeGroupReplicationClassAvailable checks if VolumeGroupReplicationClass CRD from csi-addons is available
func (d *CRDDetector) IsVolumeGroupReplicationClassAvailable(ctx context.Context) bool {
	return d.IsCRDAvailable(ctx, "volumegroupreplicationclasses.replication.storage.openshift.io")
}

// IsVolumeGroupReplicationContentAvailable checks if VolumeGroupReplicationContent CRD from csi-addons is available
func (d *CRDDetector) IsVolumeGroupReplicationContentAvailable(ctx context.Context) bool {
	return d.IsCRDAvailable(ctx, "volumegroupreplicationcontents.replication.storage.openshift.io")
}

// ClearCache clears the CRD availability cache
func (d *CRDDetector) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string]bool)
}

// Made with Bob
