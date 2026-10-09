// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
)

// This is the only seam between the VRG reconcile and the test failover. The
// dry-run package does the work; everything here does is hand it what it needs
// and write back what it reports.
//
// Keeping the seam this thin is the point. The production capture and restore
// paths are untouched by the feature, so a test failover cannot regress a real
// one, and the package can be deleted or replaced without unpicking anything.

// reconcileDryRun builds a test copy of this workload beside the real one.
//
// It replaces the in-place kube object restore rather than running alongside
// it: during a test failover the real application is never restored on this
// cluster. Only the PVCs and PVs are, and only so their volumes can be
// promoted and snapshotted.
func (v *VRGInstance) reconcileDryRun() ctrl.Result {
	in, err := v.dryRunInput()
	if err != nil {
		v.log.Error(err, "Dry run input could not be assembled")
		v.setDryRunStatus(dryrun.Output{
			State:   ramendrv1alpha1.DryRunBuildInProgress,
			Message: err.Error(),
			Requeue: true,
		})

		v.result.Requeue = true

		return v.updateVRGConditionsAndStatus(v.result)
	}

	out, err := dryrun.Reconcile(v.ctx, in)
	if err != nil {
		v.log.Error(err, "Dry run reconcile failed")

		v.result.Requeue = true

		return v.updateVRGConditionsAndStatus(v.result)
	}

	v.setDryRunStatus(out)

	if out.Requeue {
		v.result.Requeue = true
	}

	return v.updateVRGConditionsAndStatus(v.result)
}

// dryRunInput collects what the dry-run package needs. Anything that requires
// knowledge this package owns -- resolving a StorageClass to a provisioner,
// the shape of an S3 store accessor, where the DRPC UID is stamped -- is
// resolved here so the package below stays free of it.
func (v *VRGInstance) dryRunInput() (dryrun.Input, error) {
	cephFSPVCs, err := v.cephFSProtectedPVCNames()
	if err != nil {
		return dryrun.Input{}, err
	}

	profiles := make([]dryrun.S3Profile, 0, len(v.s3StoreAccessors))

	for _, accessor := range v.s3StoreAccessors {
		profiles = append(profiles, dryrun.S3Profile{
			Name:           accessor.S3ProfileName,
			Endpoint:       accessor.S3CompatibleEndpoint,
			Bucket:         accessor.S3Bucket,
			Region:         accessor.S3Region,
			SecretKeyRef:   accessor.VeleroNamespaceSecretKeyRef,
			CACertificates: accessor.CACertificates,
		})
	}

	return dryrun.Input{
		Client:          v.reconciler.Client,
		APIReader:       v.reconciler.APIReader,
		Log:             v.log,
		VRG:             v.instance,
		DRPCUID:         v.instance.GetAnnotations()[DRPCUIDAnnotation],
		RecipeElements:  v.recipeElements,
		VolRepPVCs:      v.volRepPVCs,
		CephFSPVCs:      cephFSPVCs,
		S3Profiles:      profiles,
		KubeObjects:     v.reconciler.kubeObjects,
		VeleroNamespace: v.veleroNamespaceName(),
	}, nil
}

// cephFSProtectedPVCNames names the protected PVCs the dry-run snapshot path
// skips. The hub cannot work this out -- it sees a StorageClass name, not a
// provisioner -- so the verdict is reached here and the dry-run package
// decides what to do about it.
func (v *VRGInstance) cephFSProtectedPVCNames() ([]string, error) {
	var names []string

	for idx := range v.volRepPVCs {
		pvc := &v.volRepPVCs[idx]

		storageClass, err := v.getStorageClass(pvc)
		if err != nil {
			return nil, err
		}

		if v.isCephFSProvisioner(storageClass.Provisioner) {
			names = append(names, pvc.Name)
		}
	}

	return names, nil
}

// setDryRunStatus records what the test copy run reported, preserving the
// start time across reconciles and stamping a completion time once.
func (v *VRGInstance) setDryRunStatus(out dryrun.Output) {
	status := v.instance.Status.DryRun
	if status == nil {
		now := metav1.Now()
		status = &ramendrv1alpha1.DryRunStatus{StartTime: &now}
	}

	status.RunID = out.RunID
	status.State = out.State
	status.Message = out.Message

	if len(out.NamespaceMapping) > 0 {
		status.NamespaceMapping = out.NamespaceMapping
	}

	if out.State == ramendrv1alpha1.DryRunBuildCompleted && status.CompletionTime == nil {
		now := metav1.Now()
		status.CompletionTime = &now
	}

	v.instance.Status.DryRun = status
}
