// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
	"github.com/ramendr/ramen/internal/controller/kubeobjects"
)

// This is the only seam between the VRG reconcile and the test failover. The
// dry-run package does the work; everything here does is hand it what it needs
// and write back what it reports.
//
// Keeping the seam this thin is the point. The production capture and restore
// paths are untouched by the feature, so a test failover cannot regress a real
// one, and the package can be deleted or replaced without unpicking anything.

// dryRunRequeueInterval paces a run that is waiting on something.
//
// A test failover has no timeout and no failure state by design: a run that
// cannot finish waits for a human indefinitely. That makes an immediate
// requeue the wrong tool -- a stalled run would spin against the API server
// for as long as nobody notices. Progress arrives on watch events anyway, so
// this only needs to be often enough to pick up changes nothing watches, like
// a PVC binding.
const dryRunRequeueInterval = 30 * time.Second

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

		v.result.RequeueAfter = dryRunRequeueInterval

		return v.updateVRGConditionsAndStatus(v.result)
	}

	out, err := dryrun.Reconcile(v.ctx, in)
	if err != nil {
		v.log.Error(err, "Dry run reconcile failed")

		v.result.RequeueAfter = dryRunRequeueInterval

		return v.updateVRGConditionsAndStatus(v.result)
	}

	v.setDryRunStatus(out)

	if out.Requeue {
		v.result.RequeueAfter = dryRunRequeueInterval
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

	return dryrun.Input{
		Client:          v.reconciler.Client,
		APIReader:       v.reconciler.APIReader,
		Log:             v.log,
		VRG:             v.instance,
		DRPCUID:         v.instance.GetAnnotations()[DRPCUIDAnnotation],
		RecipeElements:  v.recipeElements,
		VolRepPVCs:      v.volRepPVCs,
		CephFSPVCs:      cephFSPVCs,
		Capture:         v.dryRunCapture(),
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

// dryRunHandOff gives the storage snapshots to the test namespaces and clears
// away the run's Velero requests, so the test copy survives this cluster
// being returned to Secondary, promoted to a real failover, or deleted.
//
// It replaces the old behavior of destroying the dry-run snapshots outright,
// which would have taken the test volumes with them. A run that never reached
// a given volume is still reclaimed rather than leaked; see dryrun.HandOff.
func (v *VRGInstance) dryRunHandOff() error {
	if !v.hadDryRun() {
		return nil
	}

	in, err := v.dryRunInput()
	if err != nil {
		return err
	}

	v.log.Info("Handing the dry run snapshots to the test copy")

	return dryrun.HandOff(v.ctx, in)
}

// hadDryRun reports whether a test failover ever touched this VRG. Without it
// the hand-off would run on every real failover, since the condition that
// triggers it -- dryRun going false -- is also true for a VRG that was never
// in a dry run at all.
func (v *VRGInstance) hadDryRun() bool {
	_, present := v.instance.GetAnnotations()[ramendrv1alpha1.DryRunAnnotation]

	return present
}

// dryRunCapture locates the kube objects capture to restore the test copy
// from: the same one a real failover would use.
//
// Which capture that is lives in the source VRG in S3, and reading it needs
// the object store plumbing this package owns, so it is resolved here rather
// than in the dry-run package. Returning nil is a reason for the run to wait,
// not to fail -- the store may simply not be reachable yet.
func (v *VRGInstance) dryRunCapture() *dryrun.Capture {
	for _, accessor := range v.s3StoreAccessors {
		sourceVRG, err := v.getVRGFromS3Profile(accessor.S3ProfileName)
		if err != nil {
			v.log.Info("Dry run could not read the source VRG",
				"profile", accessor.S3ProfileName, "error", err)

			continue
		}

		capture := sourceVRG.Status.KubeObjectProtection.CaptureToRecoverFrom
		if capture == nil {
			continue
		}

		// Both names come from the shared helper in package kubeobjects, so
		// what this side looks for cannot drift from what the capture wrote.
		path, _, namePrefix := kubeobjects.CapturePathNamesAndNamePrefix(
			s3PathNamePrefix(v.instance.Namespace, v.instance.Name),
			v.instance.Namespace, v.instance.Name, capture.Number, v.reconciler.kubeObjects)

		return &dryrun.Capture{
			S3Profile: dryrun.S3Profile{
				Name:           accessor.S3ProfileName,
				Endpoint:       accessor.S3CompatibleEndpoint,
				Bucket:         accessor.S3Bucket,
				Region:         accessor.S3Region,
				SecretKeyRef:   accessor.VeleroNamespaceSecretKeyRef,
				CACertificates: accessor.CACertificates,
			},
			Path:       path,
			NamePrefix: namePrefix,
		}
	}

	return nil
}
