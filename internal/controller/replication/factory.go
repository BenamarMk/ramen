// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"

	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	neutral "github.com/ramendr/replication-storage-io-crds/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReplicationFactory creates replication objects based on CRD availability
type ReplicationFactory struct {
	detector *CRDDetector
	ctx      context.Context
}

// NewReplicationFactory creates a new factory
func NewReplicationFactory(ctx context.Context, client client.Client) *ReplicationFactory {
	return &ReplicationFactory{
		detector: NewCRDDetector(client),
		ctx:      ctx,
	}
}

// NewVolumeGroupReplication creates a new VolumeGroupReplication object
func (f *ReplicationFactory) NewVolumeGroupReplication(name, namespace string) VolumeGroupReplicationInterface {
	if f.detector.IsVolumeGroupReplicationAvailable(f.ctx) {
		return &VolrepVolumeGroupReplication{
			VolumeGroupReplication: &volrep.VolumeGroupReplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
		}
	}

	return &NeutralVolumeGroupReplication{
		VolumeGroupReplication: &neutral.VolumeGroupReplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

// NewVolumeGroupReplicationClass creates a new VolumeGroupReplicationClass object
func (f *ReplicationFactory) NewVolumeGroupReplicationClass(name string) VolumeGroupReplicationClassInterface {
	if f.detector.IsVolumeGroupReplicationClassAvailable(f.ctx) {
		return &VolrepVolumeGroupReplicationClass{
			VolumeGroupReplicationClass: &volrep.VolumeGroupReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			},
		}
	}

	return &NeutralVolumeGroupReplicationClass{
		VolumeGroupReplicationClass: &neutral.VolumeGroupReplicationClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

// NewVolumeGroupReplicationContent creates a new VolumeGroupReplicationContent object
func (f *ReplicationFactory) NewVolumeGroupReplicationContent(name string) VolumeGroupReplicationContentInterface {
	if f.detector.IsVolumeGroupReplicationContentAvailable(f.ctx) {
		return &VolrepVolumeGroupReplicationContent{
			VolumeGroupReplicationContent: &volrep.VolumeGroupReplicationContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			},
		}
	}

	return &NeutralVolumeGroupReplicationContent{
		VolumeGroupReplicationContent: &neutral.VolumeGroupReplicationContent{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

// WrapVolumeGroupReplication wraps an existing VolumeGroupReplication object
func (f *ReplicationFactory) WrapVolumeGroupReplication(obj client.Object) VolumeGroupReplicationInterface {
	if vgr, ok := obj.(*volrep.VolumeGroupReplication); ok {
		return &VolrepVolumeGroupReplication{VolumeGroupReplication: vgr}
	}
	if vgr, ok := obj.(*neutral.VolumeGroupReplication); ok {
		return &NeutralVolumeGroupReplication{VolumeGroupReplication: vgr}
	}
	return nil
}

// WrapVolumeGroupReplicationClass wraps an existing VolumeGroupReplicationClass object
func (f *ReplicationFactory) WrapVolumeGroupReplicationClass(obj client.Object) VolumeGroupReplicationClassInterface {
	if vgrc, ok := obj.(*volrep.VolumeGroupReplicationClass); ok {
		return &VolrepVolumeGroupReplicationClass{VolumeGroupReplicationClass: vgrc}
	}
	if vgrc, ok := obj.(*neutral.VolumeGroupReplicationClass); ok {
		return &NeutralVolumeGroupReplicationClass{VolumeGroupReplicationClass: vgrc}
	}
	return nil
}

// WrapVolumeGroupReplicationContent wraps an existing VolumeGroupReplicationContent object
func (f *ReplicationFactory) WrapVolumeGroupReplicationContent(obj client.Object) VolumeGroupReplicationContentInterface {
	if vgrc, ok := obj.(*volrep.VolumeGroupReplicationContent); ok {
		return &VolrepVolumeGroupReplicationContent{VolumeGroupReplicationContent: vgrc}
	}
	if vgrc, ok := obj.(*neutral.VolumeGroupReplicationContent); ok {
		return &NeutralVolumeGroupReplicationContent{VolumeGroupReplicationContent: vgrc}
	}
	return nil
}

// IsUsingVolrep returns true if volrep CRDs are available
func (f *ReplicationFactory) IsUsingVolrep() bool {
	return f.detector.IsVolumeGroupReplicationAvailable(f.ctx)
}

// IsUsingNeutral returns true if neutral CRDs are being used
func (f *ReplicationFactory) IsUsingNeutral() bool {
	return !f.IsUsingVolrep()
}

// GetVolumeGroupReplicationType returns the client.Object type for VolumeGroupReplication
func (f *ReplicationFactory) GetVolumeGroupReplicationType() client.Object {
	if f.IsUsingVolrep() {
		return &volrep.VolumeGroupReplication{}
	}
	return &neutral.VolumeGroupReplication{}
}

// GetVolumeGroupReplicationClassType returns the client.Object type for VolumeGroupReplicationClass
func (f *ReplicationFactory) GetVolumeGroupReplicationClassType() client.Object {
	if f.IsUsingVolrep() {
		return &volrep.VolumeGroupReplicationClass{}
	}
	return &neutral.VolumeGroupReplicationClass{}
}

// GetVolumeGroupReplicationContentType returns the client.Object type for VolumeGroupReplicationContent
func (f *ReplicationFactory) GetVolumeGroupReplicationContentType() client.Object {
	if f.IsUsingVolrep() {
		return &volrep.VolumeGroupReplicationContent{}
	}
	return &neutral.VolumeGroupReplicationContent{}
}

// Made with Bob
