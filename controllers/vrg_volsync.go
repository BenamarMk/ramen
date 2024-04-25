// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/controllers/util"
	"github.com/ramendr/ramen/controllers/volsync"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	FinalSyncPVCNameSuffix = "-for-final-sync"
)

func (v *VRGInstance) restorePVsAndPVCsForVolSync() (int, error) {
	v.log.Info("VolSync: Restoring VolSync PVs")

	if len(v.instance.Spec.VolSync.RDSpec) == 0 {
		v.log.Info("No RDSpec entries. There are no PVCs to restore")
		// No ReplicationDestinations (i.e. no PVCs) to restore
		return 0, nil
	}

	numPVsRestored := 0

	for _, rdSpec := range v.instance.Spec.VolSync.RDSpec {
		failoverAction := v.instance.Spec.Action == ramendrv1alpha1.VRGActionFailover
		// Create a PVC from snapshot or for direct copy
		err := v.volSyncHandler.EnsurePVCfromRD(rdSpec, failoverAction)
		if err != nil {
			v.log.Info(fmt.Sprintf("Unable to ensure PVC %v -- err: %v", rdSpec, err))

			protectedPVC := v.findFirstProtectedPVCWithName(rdSpec.ProtectedPVC.Name)
			if protectedPVC == nil {
				protectedPVC = &ramendrv1alpha1.ProtectedPVC{}
				rdSpec.ProtectedPVC.DeepCopyInto(protectedPVC)
				v.instance.Status.ProtectedPVCs = append(v.instance.Status.ProtectedPVCs, *protectedPVC)
			}

			setVRGConditionTypeVolSyncPVRestoreError(&protectedPVC.Conditions, v.instance.Generation,
				fmt.Sprintf("%v", err))

			continue // Keep trying to ensure PVCs for other rdSpec
		}

		numPVsRestored++

		protectedPVC := v.findFirstProtectedPVCWithName(rdSpec.ProtectedPVC.Name)
		if protectedPVC == nil {
			protectedPVC = &ramendrv1alpha1.ProtectedPVC{}
			rdSpec.ProtectedPVC.DeepCopyInto(protectedPVC)
			v.instance.Status.ProtectedPVCs = append(v.instance.Status.ProtectedPVCs, *protectedPVC)
		}

		setVRGConditionTypeVolSyncPVRestoreComplete(&protectedPVC.Conditions, v.instance.Generation, "PVC restored")
	}

	if numPVsRestored != len(v.instance.Spec.VolSync.RDSpec) {
		return numPVsRestored, fmt.Errorf("failed to restore all PVCs using RDSpec (%v)", v.instance.Spec.VolSync.RDSpec)
	}

	v.log.Info("Success restoring VolSync PVs", "Total", numPVsRestored)

	return numPVsRestored, nil
}

func (v *VRGInstance) reconcileVolSyncAsPrimary(finalSyncPrepared *bool) (requeue bool) {
	finalSyncComplete := func() {
		*finalSyncPrepared = true
		v.instance.Status.FinalSyncComplete = v.instance.Spec.RunFinalSync
	}

	if len(v.volSyncPVCs) == 0 {
		finalSyncComplete()

		return
	}

	v.log.Info(fmt.Sprintf("Reconciling VolSync as Primary. %d VolSyncPVCs", len(v.volSyncPVCs)))

	// Cleanup - this VRG is primary, cleanup if necessary
	// remove any ReplicationDestinations (that would have been created when this VRG was secondary) if they
	// are not in the RDSpec list
	if err := v.volSyncHandler.CleanupRDNotInSpecList(v.instance.Spec.VolSync.RDSpec); err != nil {
		v.log.Error(err, "Failed to cleanup the RDSpecs when this VRG instance was secondary")

		requeue = true

		return
	}

	for _, pvc := range v.volSyncPVCs {
		requeuePVC := v.reconcilePVCAsVolSyncPrimary(pvc)
		if requeuePVC {
			requeue = true
		}
	}

	if requeue {
		v.log.Info("Not all ReplicationSources completed setup. We'll retry...")

		return requeue
	}

	finalSyncComplete()
	v.log.Info("Successfully reconciled VolSync as Primary")

	return requeue
}

func (v *VRGInstance) reconcilePVCAsVolSyncPrimary(pvc corev1.PersistentVolumeClaim) bool {
	newProtectedPVC := &ramendrv1alpha1.ProtectedPVC{
		Name:               pvc.Name,
		Namespace:          pvc.Namespace,
		ProtectedByVolSync: true,
		StorageClassName:   pvc.Spec.StorageClassName,
		Annotations:        protectedPVCAnnotations(pvc),
		Labels:             pvc.Labels,
		AccessModes:        pvc.Spec.AccessModes,
		Resources:          pvc.Spec.Resources,
	}

	protectedPVC := v.findFirstProtectedPVCWithName(pvc.Name)
	if protectedPVC == nil {
		protectedPVC = newProtectedPVC
		v.instance.Status.ProtectedPVCs = append(v.instance.Status.ProtectedPVCs, *protectedPVC)
	} else if !reflect.DeepEqual(protectedPVC, newProtectedPVC) {
		newProtectedPVC.Conditions = protectedPVC.Conditions
		newProtectedPVC.DeepCopyInto(protectedPVC)
	}

	// Not much need for VolSyncReplicationSourceSpec anymore - but keeping it around in case we want
	// to add anything to it later to control anything in the ReplicationSource
	rsSpec := ramendrv1alpha1.VolSyncReplicationSourceSpec{
		ProtectedPVC: *protectedPVC,
	}

	const requeue = true

	err := v.protectPVCAndRetainPV(&pvc)
	if err != nil {
		return requeue
	}

	// reconcile RS and if runFinalSync is true, then one final sync will be run
	_, rs, err := v.volSyncHandler.ReconcileRS(rsSpec, false)
	if err != nil {
		v.log.Info(fmt.Sprintf("Failed to reconcile VolSync Replication Source for rsSpec %v. Error %v",
			rsSpec, err))

		setVRGConditionTypeVolSyncRepSourceSetupError(&protectedPVC.Conditions, v.instance.Generation,
			"VolSync setup failed")

		return requeue
	}

	if rs == nil {
		return requeue
	}

	setVRGConditionTypeVolSyncRepSourceSetupComplete(&protectedPVC.Conditions, v.instance.Generation, "Ready")

	return !requeue
}

func (v *VRGInstance) protectPVCAndRetainPV(pvc *corev1.PersistentVolumeClaim) error {
	// Add VolSync finalizer to PVC for deletion protection
	err := util.NewResourceUpdater(pvc).
		AddFinalizer(PvcVSFinalizerProtected).
		Update(v.ctx, v.reconciler.Client)

	if err != nil {
		return err
	}

	return util.RetainPVAndUpdate(v.ctx, v.reconciler.Client, util.PVAnnotationRetainedForVolSync, pvc, v.log)
}

func (v *VRGInstance) reconcileVolSyncAsSecondary() bool {
	v.log.Info("Reconcile VolSync as Secondary", "RDSpec", v.instance.Spec.VolSync.RDSpec)

	// Ensure final sync if changing to secondary due to a relocate request and RDSpec has not been set yet.
	if v.instance.Spec.Action == ramendrv1alpha1.VRGActionRelocate && v.instance.Spec.VolSync.RDSpec == nil {
		return v.prepareAndReconcileForFinalSync()
	}

	return v.reconcileRDSpecForDeletionOrReplication()
}

func (v *VRGInstance) prepareAndReconcileForFinalSync() bool {
	v.log.Info("Reconcile VolSync as Secondary", "RDSpec", v.instance.Spec.VolSync.RDSpec)
	for _, appPVC := range v.volSyncPVCs {
		// Create the tmp pvc with the application pvc name and an internal suffix
		tmpPVCName := appPVC.GetName() + FinalSyncPVCNameSuffix
		tmpPVC, err := v.prepareFinalSync(&appPVC, tmpPVCName, v.log)
		if err != nil {
			v.log.Info("Failed to prepare for final sync.", "Error", err)

			continue
		}

		// Run finalsync
		err = v.reconcileForFinalSync(tmpPVC)
		if err != nil {
			v.log.Info("Reconciled for final sync", "Error", err)

			continue
		}

		// Prepared and ran final sync successfully. Remove APP PVC finalizer.
		err = v.cleanupAfterFinalSync(tmpPVC)
		if err != nil {
			v.log.Info("Final sync cleanup", "Error", err)

			return true // requeue
		}
	}

	return false // don't requeue
}

// prepareFinalSync will do the following:
// 1. Retain the PV claimed by the PVC
// 2. Updates the ClaimRef to point to a new temporary PVC
// 3. Create the temporary PVC
func (v *VRGInstance) prepareFinalSync(pvc *corev1.PersistentVolumeClaim,
	tmpPVCName string,
	log logr.Logger,
) (*corev1.PersistentVolumeClaim, error) {
	if err := v.updatePVForFinalSync(pvc, tmpPVCName, log); err != nil {
		return nil, err
	}

	return v.createAndPrepareTmpPVForFinalSync(pvc, tmpPVCName, log)
}

// updatePVForFinalSync will retain the PV and changes the claimRef to point to a new PVC
func (v *VRGInstance) updatePVForFinalSync(pvc *corev1.PersistentVolumeClaim, tmpPVCName string, log logr.Logger,
) error {
	return v.setPVReclaimPolicy(pvc, tmpPVCName, corev1.PersistentVolumeReclaimRetain, log)
}

func (v *VRGInstance) cleanupAfterFinalSync(pvc *corev1.PersistentVolumeClaim) error {
	v.log.Info("Reset after final sync is complete")
	// Prepared and ran final sync successfully. Remove APP PVC finalizer.
	err := util.NewResourceUpdater(pvc).
		RemoveFinalizer(PvcVSFinalizerProtected).
		Update(v.ctx, v.reconciler.Client)
	if err != nil {
		return err // requeue
	}

	err = v.setPVReclaimPolicy(pvc, pvc.GetName(), corev1.PersistentVolumeReclaimDelete, v.log)
	if err != nil {
		return err
	}

	return v.reconciler.Client.Delete(v.ctx, pvc)
}

func (v *VRGInstance) setPVReclaimPolicy(pvc *corev1.PersistentVolumeClaim, claimName string,
	reclaimPolicy corev1.PersistentVolumeReclaimPolicy, log logr.Logger,
) error {
	pv := &corev1.PersistentVolume{}
	pvObjectKey := client.ObjectKey{
		Name: pvc.Spec.VolumeName,
	}

	if err := v.reconciler.Client.Get(v.ctx, pvObjectKey, pv); err != nil {
		log.Error(err, "Failed to get PersistentVolume", "volumeName", pvc.Spec.VolumeName)

		return err
	}

	v.log.Info("Updating PV", "Name", pv.GetName(), "ReclaimPolicy", reclaimPolicy, "ClaimRef", claimName)
	// Change reclaim policy if it has not been set earlier (on becoming primary)
	ChangeReclaimPolicy(pv, reclaimPolicy, util.PVAnnotationRetainedForVolSync)
	// Update the claimRef to the new tmp PVC.
	ChangeClaimRef(pv, claimName)

	if err := v.reconciler.Client.Update(v.ctx, pv); err != nil {
		log.Error(err, "Failed to update PersistentVolume")

		return fmt.Errorf("failed to update PersistentVolume resource (%s) reclaim policy for"+
			" PersistentVolumeClaim resource (%s/%s), %w",
			pvc.Spec.VolumeName, pvc.Namespace, pvc.Name, err)
	}

	return nil
}

func (v *VRGInstance) createAndPrepareTmpPVForFinalSync(pvc *corev1.PersistentVolumeClaim, tmpPVCName string, log logr.Logger,
) (*corev1.PersistentVolumeClaim, error) {
	tmpPVC := pvc.DeepCopy()
	tmpPVC.ObjectMeta.Name = tmpPVCName
	cleanupPVCForRestore(tmpPVC)

	op, err := ctrlutil.CreateOrUpdate(v.ctx, v.reconciler.Client, tmpPVC, func() error {
		if err := ctrl.SetControllerReference(v.instance, tmpPVC, v.reconciler.Client.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	log.V(1).Info("Temporary PVC", "operation", op)

	return tmpPVC, nil
}

func ChangeReclaimPolicy(pv *corev1.PersistentVolume,
	newReclaimPolicy corev1.PersistentVolumeReclaimPolicy,
	reclaimer string,
) bool {
	const updated = true
	// Check reclaimPolicy of PV, if already set to retain
	if pv.Spec.PersistentVolumeReclaimPolicy == newReclaimPolicy {
		return !updated
	}

	// if not retained, retain PV, and add an annotation to denote this is updated for VolRep/VolSync
	pv.Spec.PersistentVolumeReclaimPolicy = newReclaimPolicy
	if pv.ObjectMeta.Annotations == nil {
		pv.ObjectMeta.Annotations = map[string]string{}
	}

	pv.ObjectMeta.Annotations[util.PVAnnotationRetainKey] = reclaimer

	return updated
}

func ChangeClaimRef(pv *corev1.PersistentVolume, pvcName string) {
	pv.Spec.ClaimRef.Name = pvcName
	preparePVForReclaim(pv)
}

func (v *VRGInstance) reconcileForFinalSync(pvc *corev1.PersistentVolumeClaim) error {
	v.log.Info("Reconcile final sync")
	protectedPVC := &ramendrv1alpha1.ProtectedPVC{
		Name:               pvc.GetName(),
		Namespace:          pvc.GetNamespace(),
		ProtectedByVolSync: true,
		StorageClassName:   pvc.Spec.StorageClassName,
		Annotations:        protectedPVCAnnotations(*pvc),
		Labels:             pvc.Labels,
		AccessModes:        pvc.Spec.AccessModes,
		Resources:          pvc.Spec.Resources,
	}

	rsSpec := ramendrv1alpha1.VolSyncReplicationSourceSpec{
		ProtectedPVC: *protectedPVC,
	}

	runFinalSync := true
	// reconcile RS for FinalSync
	finalSyncComplete, _, err := v.volSyncHandler.ReconcileRS(rsSpec, runFinalSync)
	if err != nil {
		v.log.Info(fmt.Sprintf("Failed to reconcile VolSync Replication Source for rsSpec %v. Error %v",
			rsSpec, err))

		return err
	}

	if !finalSyncComplete {
		return fmt.Errorf("waiting for finalSync to complete")
	}

	v.log.Info("Final sysnc completed")

	return nil
}

/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////////////////////////////

func (v *VRGInstance) reconcileRDSpecForDeletionOrReplication() bool {
	requeue := false

	for _, rdSpec := range v.instance.Spec.VolSync.RDSpec {
		v.log.Info("Reconcile RD as Secondary", "RDSpec", rdSpec)

		rd, err := v.volSyncHandler.ReconcileRD(rdSpec)
		if err != nil {
			v.log.Error(err, "Failed to reconcile VolSync Replication Destination")

			requeue = true

			break
		}

		if rd == nil {
			v.log.Info(fmt.Sprintf("ReconcileRD - ReplicationDestination for %s is not ready. We'll retry...",
				rdSpec.ProtectedPVC.Name))

			requeue = true
		}
	}

	if !requeue {
		v.log.Info("Successfully reconciled VolSync as Secondary")
	}

	return requeue
}

func (v *VRGInstance) aggregateVolSyncDataReadyCondition() *metav1.Condition {
	dataReadyCondition := &metav1.Condition{
		Status:             metav1.ConditionTrue,
		Type:               VRGConditionTypeDataReady,
		Reason:             VRGConditionReasonReady,
		ObservedGeneration: v.instance.Generation,
		Message:            "All VolSync PVCs are ready",
	}

	if v.instance.Spec.ReplicationState == ramendrv1alpha1.Secondary {
		dataReadyCondition.Reason = VRGConditionReasonUnused
		dataReadyCondition.Message = "Volsync based PVC protection does not report DataReady condition as Secondary"

		return dataReadyCondition
	}

	if len(v.volSyncPVCs) == 0 {
		dataReadyCondition.Reason = VRGConditionReasonUnused
		dataReadyCondition.Message = "No PVCs are protected using Volsync scheme"

		return dataReadyCondition
	}

	// On Failover/Relocation, we depend on PVs to be restored. For initial deployment,
	// we depend on ReplicationSourceSetup to determine Data readiness.
	ready := v.isVolSyncProtectedPVCConditionReady(VRGConditionTypeVolSyncPVsRestored) ||
		v.isVolSyncProtectedPVCConditionReady(VRGConditionTypeVolSyncRepSourceSetup)

	if !ready {
		dataReadyCondition.Status = metav1.ConditionFalse
		dataReadyCondition.Message = "Not all VolSync PVCs are ready"
		dataReadyCondition.Reason = VRGConditionReasonProgressing

		return dataReadyCondition
	}

	return dataReadyCondition
}

func (v *VRGInstance) aggregateVolSyncDataProtectedConditions() (*metav1.Condition, *metav1.Condition) {
	// For VolSync, clusterDataProtectedCondition is the same as dataProtectedCondition - so copy it
	dataProtectedCondition := v.buildDataProtectedCondition()

	if dataProtectedCondition == nil {
		return nil, nil
	}

	clusterDataProtectedCondition := dataProtectedCondition.DeepCopy()
	clusterDataProtectedCondition.Type = VRGConditionTypeClusterDataProtected

	return dataProtectedCondition, clusterDataProtectedCondition
}

//nolint:gocognit,funlen,cyclop
func (v *VRGInstance) buildDataProtectedCondition() *metav1.Condition {
	if len(v.volSyncPVCs) == 0 && len(v.instance.Spec.VolSync.RDSpec) == 0 {
		return newVRGAsDataProtectedUnusedCondition(v.instance.Generation,
			"No PVCs are protected using Volsync scheme")
	}

	if v.instance.Spec.ReplicationState == ramendrv1alpha1.Secondary {
		// The primary will contain the DataProtected condition.
		return newVRGAsDataProtectedUnusedCondition(v.instance.Generation,
			"Volsync based PVC protection does not report DataProtected/ClusterDataProtected conditions as Secondary")
	}

	ready := true

	protectedByVolSyncCount := 0

	//nolint:nestif
	for _, protectedPVC := range v.instance.Status.ProtectedPVCs {
		if protectedPVC.ProtectedByVolSync {
			protectedByVolSyncCount++

			condition := findCondition(protectedPVC.Conditions, VRGConditionTypeVolSyncRepSourceSetup)
			if condition == nil || condition.Status != metav1.ConditionTrue {
				ready = false

				v.log.Info(fmt.Sprintf("VolSync RS hasn't been setup yet for PVC %s", protectedPVC.Name))

				break
			}

			// IFF however, we are running the final sync, then we have to wait
			condition = findCondition(protectedPVC.Conditions, VRGConditionTypeVolSyncFinalSyncInProgress)
			if condition != nil && condition.Status != metav1.ConditionTrue {
				ready = false

				v.log.Info(fmt.Sprintf("VolSync RS is in progress for PVC %s", protectedPVC.Name))

				break
			}

			// Check now if we have synced up at least once for this PVC
			rsDataProtected, err := v.volSyncHandler.IsRSDataProtected(protectedPVC.Name)
			if err != nil || !rsDataProtected {
				ready = false

				v.log.Info(fmt.Sprintf("First sync has not yet completed for VolSync RS %s -- Err %v",
					protectedPVC.Name, err))

				break
			}
		}
	}

	if ready && len(v.volSyncPVCs) > protectedByVolSyncCount {
		ready = false

		v.log.Info(fmt.Sprintf("VolSync PVCs count does not match with the ready PVCs %d/%d",
			len(v.volSyncPVCs), protectedByVolSyncCount))
	}

	dataProtectedCondition := &metav1.Condition{
		Type:               VRGConditionTypeDataProtected,
		ObservedGeneration: v.instance.Generation,
	}

	if !ready {
		dataProtectedCondition.Status = metav1.ConditionFalse
		dataProtectedCondition.Reason = VRGConditionReasonProgressing
		dataProtectedCondition.Message = "Not all VolSync PVCs are protected"
	} else {
		dataProtectedCondition.Status = metav1.ConditionTrue
		dataProtectedCondition.Reason = VRGConditionReasonDataProtected
		dataProtectedCondition.Message = "All VolSync PVCs are protected"
	}

	return dataProtectedCondition
}

func (v VRGInstance) isVolSyncProtectedPVCConditionReady(conType string) bool {
	ready := len(v.instance.Status.ProtectedPVCs) != 0

	for _, protectedPVC := range v.instance.Status.ProtectedPVCs {
		if protectedPVC.ProtectedByVolSync {
			condition := findCondition(protectedPVC.Conditions, conType)
			if condition == nil || condition.Status != metav1.ConditionTrue {
				ready = false

				v.log.Info(fmt.Sprintf("VolSync %s is not complete yet for PVC %s", conType, protectedPVC.Name))

				break
			}
		}
	}

	return ready
}

// protectedPVCAnnotations return the annotations that we must propagate to the
// destination cluster:
//   - apps.open-cluster-management.io/* - required to make the protected PVC
//     owned by OCM when DR is disabled. Copy all annnotations except the
//     special "do-not-delete" annotation, used only on the source cluster
//     during relocate.
func protectedPVCAnnotations(pvc corev1.PersistentVolumeClaim) map[string]string {
	res := map[string]string{}

	for key, value := range pvc.Annotations {
		if strings.HasPrefix(key, "apps.open-cluster-management.io/") &&
			key != volsync.ACMAppSubDoNotDeleteAnnotation {
			res[key] = value
		}
	}

	return res
}

func (v *VRGInstance) pvcUnprotectVolSync(pvc corev1.PersistentVolumeClaim, log logr.Logger) {
	if !VolumeUnprotectionEnabledForAsyncVolSync {
		log.Info("Volume unprotection disabled for VolSync")

		return
	}
	// TODO Delete ReplicationSource, ReplicationDestination, etc.
	v.pvcStatusDeleteIfPresent(pvc.Namespace, pvc.Name, log)
}

// disownPVCs this function is disassociating all PVCs (targeted for VolSync replication) from its owner (VRG)
func (v *VRGInstance) disownPVCs() error {
	if v.instance.GetAnnotations()[DoNotDeletePVCAnnotation] != DoNotDeletePVCAnnotationVal {
		return nil
	}

	for idx := range v.volSyncPVCs {
		pvc := &v.volSyncPVCs[idx]

		err := v.volSyncHandler.DisownVolSyncManagedPVC(pvc)
		if err != nil {
			return err
		}
	}

	return nil
}
