// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"fmt"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
	"github.com/ramendr/ramen/internal/controller/kubeobjects"
	rmnutil "github.com/ramendr/ramen/internal/controller/util"
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

	v.log.Info("Reconciling the dry run test copy",
		"protectedNamespaces", v.instance.Spec.ProtectedNamespaces,
		"volRepPVCs", len(in.VolRepPVCs), "cephFSPVCs", in.CephFSPVCs,
		"capture", in.Capture != nil)

	out, err := dryrun.Reconcile(v.ctx, in)
	if err != nil {
		v.log.Error(err, "Dry run reconcile failed")

		v.result.RequeueAfter = dryRunRequeueInterval

		return v.updateVRGConditionsAndStatus(v.result)
	}

	v.setDryRunStatus(out)

	v.log.Info("Dry run reconcile done",
		"state", out.State, "message", out.Message, "requeue", out.Requeue)

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
			v.log.Info("Dry run found no capture to restore from",
				"profile", accessor.S3ProfileName)

			continue
		}

		// Both names come from the shared helper in package kubeobjects, so
		// what this side looks for cannot drift from what the capture wrote.
		path, _, namePrefix := kubeobjects.CapturePathNamesAndNamePrefix(
			s3PathNamePrefix(v.instance.Namespace, v.instance.Name),
			v.instance.Namespace, v.instance.Name, capture.Number, v.reconciler.kubeObjects)

		v.log.Info("Dry run restoring from capture",
			"profile", accessor.S3ProfileName, "number", capture.Number,
			"capturedAt", capture.EndTime, "path", path)

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

// dryRunDiscardRestoredPVCs deletes the PVCs a test failover restored into
// the protected namespaces, so this cluster can go back to Secondary.
//
// Nothing else is going to delete them. Demoting a cluster waits for its
// PVCs to be marked for deletion, and for a discovered application that
// means waiting for the user to remove the workload. But on a test cluster
// the user never deployed anything: Ramen restored these PVCs itself, from
// S3, purely so their volumes could be promoted and snapshotted. Waiting for
// someone to clean up something they never created leaves the DRPC in
// WaitOnUserToCleanUp for good.
//
// Only the PVCs in the protected namespaces go. The test copy's own PVCs
// live in the generated namespaces, are not selected by the VRG, and are
// left running -- that is the whole point of the feature.
func (v *VRGInstance) dryRunDiscardRestoredPVCs() error {
	// status.dryRun is written only by the cluster that built a test copy.
	// The annotation is not a safe substitute: it is copied onto every VRG
	// the DRPC manages, including the one running the real workload, whose
	// PVCs must never be touched here.
	status := v.instance.Status.DryRun
	if status == nil {
		return nil
	}

	// Promotion to a real failover keeps them. They stop being scaffolding
	// for a test copy and become the workload's own volumes.
	if v.instance.Spec.ReplicationState != ramendrv1alpha1.Secondary {
		return nil
	}

	for idx := range v.volRepPVCs {
		pvc := &v.volRepPVCs[idx]

		if _, restored := status.NamespaceMapping[pvc.Namespace]; !restored {
			continue
		}

		if rmnutil.ResourceIsDeleted(pvc) {
			continue
		}

		v.log.Info("Deleting a PVC the dry run restored, so this cluster can be demoted",
			"pvc", pvc.Name, "namespace", pvc.Namespace, "runID", status.RunID)

		if err := v.reconciler.Client.Delete(v.ctx, pvc); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("deleting the restored PVC %s/%s: %w", pvc.Namespace, pvc.Name, err)
		}
	}

	return nil
}

// dryRunTearDown deletes the test copy once the user has finished with it,
// and clears status.dryRun when it is gone.
//
// Clearing the status is how the hub learns the copy is no longer there: it
// is the same field that says a test copy exists, so emptying it is what
// frees the DRPC to run another test.
func (v *VRGInstance) dryRunTearDown() error {
	if v.dryRunPhase() != ramendrv1alpha1.DryRunTearingDown {
		return nil
	}

	if v.instance.Status.DryRun == nil {
		return nil
	}

	in, err := v.dryRunInput()
	if err != nil {
		return err
	}

	done, err := dryrun.TearDown(v.ctx, in)
	if err != nil {
		return err
	}

	if !done {
		// Paced like every other dry-run wait, and deliberately not
		// v.requeue(): Requeue: true is rate limited and the item is never
		// forgotten, so the delay doubles on every pass. Namespaces take
		// tens of seconds to finish terminating, which is long enough for
		// that to escalate well past the interval wanted here.
		v.result.RequeueAfter = dryRunRequeueInterval

		return nil
	}

	v.instance.Status.DryRun = nil

	return nil
}

// dryRunPhase is the lifecycle phase the hub has put this run in, carried on
// the VRG by the same annotation it sets on the DRPC.
func (v *VRGInstance) dryRunPhase() ramendrv1alpha1.DryRunPhase {
	return ramendrv1alpha1.DryRunPhase(v.instance.GetAnnotations()[ramendrv1alpha1.DryRunAnnotation])
}
