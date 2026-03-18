// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ramendr/ramen/internal/controller/replication"
	rmnutil "github.com/ramendr/ramen/internal/controller/util"
)

// This file contains dynamic CRD-aware helper methods that work with both
// volrep and neutral VolumeGroupReplication types using the factory pattern.
// These methods complement the existing volrep-specific methods in vrg_volgrouprep.go

// getVGRDynamic retrieves a VolumeGroupReplication using the factory
func (v *VRGInstance) getVGRDynamic(vrNamespacedName types.NamespacedName) (replication.VolumeGroupReplicationInterface, error) {
	vgrObj := v.replicationFactory.GetVolumeGroupReplicationType()
	
	if err := v.reconciler.Get(v.ctx, vrNamespacedName, vgrObj); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplication %v: %w", vrNamespacedName, err)
	}
	
	return v.replicationFactory.WrapVolumeGroupReplication(vgrObj), nil
}

// getVGRCFromVGRDynamic retrieves a VolumeGroupReplicationContent from a VGR using the factory
func (v *VRGInstance) getVGRCFromVGRDynamic(vgr replication.VolumeGroupReplicationInterface) (replication.VolumeGroupReplicationContentInterface, error) {
	vgrcName := vgr.GetSpec().GetVolumeGroupReplicationContentName()
	vgrcObjectKey := client.ObjectKey{Name: vgrcName}
	
	vgrcObj := v.replicationFactory.GetVolumeGroupReplicationContentType()
	if err := v.reconciler.Get(v.ctx, vgrcObjectKey, vgrcObj); err != nil {
		return nil, fmt.Errorf("failed to get VGRC %v from VGR %v: %w",
			vgrcObjectKey, client.ObjectKeyFromObject(vgr), err)
	}
	
	return v.replicationFactory.WrapVolumeGroupReplicationContent(vgrcObj), nil
}

// getVGRClassDynamic retrieves a VolumeGroupReplicationClass using the factory
func (v *VRGInstance) getVGRClassDynamic(className string) (replication.VolumeGroupReplicationClassInterface, error) {
	vgrClassObj := v.replicationFactory.GetVolumeGroupReplicationClassType()
	
	if err := v.reconciler.Get(v.ctx, types.NamespacedName{Name: className}, vgrClassObj); err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupReplicationClass %s: %w", className, err)
	}
	
	return v.replicationFactory.WrapVolumeGroupReplicationClass(vgrClassObj), nil
}

// deleteVGRDynamic deletes a VolumeGroupReplication using the factory
func (v *VRGInstance) deleteVGRDynamic(vrNamespacedName types.NamespacedName, log logr.Logger) error {
	vgrObj := v.replicationFactory.GetVolumeGroupReplicationType()
	vgrObj.SetName(vrNamespacedName.Name)
	vgrObj.SetNamespace(vrNamespacedName.Namespace)
	
	if err := v.reconciler.Delete(v.ctx, vgrObj); err != nil {
		return fmt.Errorf("failed to delete VolumeGroupReplication %v: %w", vrNamespacedName, err)
	}
	
	log.Info("Deleted VolumeGroupReplication resource", "name", vrNamespacedName.Name, "namespace", vrNamespacedName.Namespace)
	
	return nil
}

// isVGRandVGRCArchivedAlreadyDynamic checks if VGR and VGRC are archived using the factory
func (v *VRGInstance) isVGRandVGRCArchivedAlreadyDynamic(vgr replication.VolumeGroupReplicationInterface, log logr.Logger) bool {
	vgrc, err := v.getVGRCFromVGRDynamic(vgr)
	if err != nil {
		log.Error(err, "Failed to get VGRC to check if archived")
		return false
	}
	
	if vgr.GetAnnotations()[pvcVRAnnotationArchivedKey] != v.generateArchiveAnnotation(vgr.GetGeneration()) {
		return false
	}
	
	if vgrc.GetAnnotations()[pvcVRAnnotationArchivedKey] != v.generateArchiveAnnotation(vgrc.GetGeneration()) {
		return false
	}
	
	return true
}

// ensurePVCUnprotectedDynamic checks if a PVC is unprotected from a VGR using the factory
func (v *VRGInstance) ensurePVCUnprotectedDynamic(
	pvc *corev1.PersistentVolumeClaim,
	vgr replication.VolumeGroupReplicationInterface,
) bool {
	const unprotected = true
	
	// Check if VGR is being deleted
	if vgr.GetDeletionTimestamp() != nil {
		return !unprotected
	}
	
	// Check if PVC is in the VGR's status
	status := vgr.GetStatus()
	pvcRefs := status.GetPersistentVolumeClaimsRefList()
	
	for _, pvcRef := range pvcRefs {
		if pvcRef.Name == pvc.GetName() {
			return !unprotected
		}
	}
	
	return unprotected
}

// getVGRUsingSCLabelDynamic retrieves a VGR using storage class label with the factory
func (v *VRGInstance) getVGRUsingSCLabelDynamic(pvc *corev1.PersistentVolumeClaim) (replication.VolumeGroupReplicationInterface, error) {
	grID, err := v.getVGRClassReplicationID(pvc)
	if err != nil {
		return nil, fmt.Errorf("error determining replicationID")
	}
	
	vgrNamespacedName := types.NamespacedName{
		Name:      rmnutil.CreateVGRName(grID, v.instance.Name),
		Namespace: pvc.Namespace,
	}
	
	return v.getVGRDynamic(vgrNamespacedName)
}

// Helper function to check if VGR CRDs are available
func (v *VRGInstance) areVGRCRDsAvailable() bool {
	return v.replicationFactory.IsUsingVolrep() || v.replicationFactory.IsUsingNeutral()
}

// Helper function to log which CRD implementation is being used
func (v *VRGInstance) logActiveCRDImplementation(log logr.Logger) {
	if v.replicationFactory.IsUsingVolrep() {
		log.Info("Using volrep VolumeGroupReplication CRDs (replication.storage.openshift.io)")
	} else if v.replicationFactory.IsUsingNeutral() {
		log.Info("Using neutral VolumeGroupReplication CRDs (replication.storage.io)")
	} else {
		log.Info("No VolumeGroupReplication CRDs available")
	}
}

// Made with Bob
