// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"fmt"
	"strings"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandOff tears down the source side of a test failover without taking the
// test copy with it.
//
// It runs whenever a run ends: on the revert, on a promotion to a real
// failover, and from the VRG delete path. All three are the same problem --
// the source objects are about to go away, and the storage snapshots behind
// the test volumes must not go with them.
//
// It tolerates a run that only got part way, because a run can be pre-empted
// or aborted at any point.
func HandOff(ctx context.Context, in Input) error {
	runID := RunID(in.DRPCUID)
	in.Log = in.Log.WithValues("runID", runID)

	in.Log.Info("Handing the dry run source objects over")

	snapshots := &snapv1.VolumeSnapshotList{}
	if err := in.APIReader.List(ctx, snapshots,
		client.MatchingLabels(SnapshotSelector(in.VRG.GetName()))); err != nil {
		return fmt.Errorf("listing dry run snapshots: %w", err)
	}

	in.Log.Info("Dry run snapshots found", "count", len(snapshots.Items))

	for idx := range snapshots.Items {
		snapshot := &snapshots.Items[idx]

		// Snapshots already in a test namespace belong to the copy and stay.
		if snapshot.Labels[NamespaceOfLabel] != "" {
			in.Log.V(1).Info("Leaving a test namespace snapshot alone",
				"snapshot", snapshot.Name, "namespace", snapshot.Namespace)

			continue
		}

		if err := handOffSnapshot(ctx, in, runID, snapshot); err != nil {
			return err
		}
	}

	if err := deleteRequests(ctx, in, runID); err != nil {
		return err
	}

	in.Log.Info("Dry run source objects handed over")

	return nil
}

// handOffSnapshot disposes of one source snapshot according to whether the
// test copy ever took ownership of the storage behind it.
func handOffSnapshot(
	ctx context.Context, in Input, runID string, snapshot *snapv1.VolumeSnapshot,
) error {
	pvcName := strings.TrimSuffix(snapshot.Name, "-snapshot")

	adopted, err := importedContentExists(ctx, in, runID, pvcName)
	if err != nil {
		return err
	}

	if adopted {
		// The test namespace owns the storage snapshot now, through an
		// imported content with a Delete policy. Deleting the source objects
		// must leave the storage alone, which the Retain policy set during
		// the import guarantees.
		in.Log.Info("Leaving the storage snapshot to the test copy",
			"pvc", pvcName, "snapshot", snapshot.Name)
	} else {
		// Nothing took ownership -- the run was aborted before this volume
		// was handed over. Let the source reclaim the storage rather than
		// leaking a snapshot nothing references.
		in.Log.Info("Reclaiming a storage snapshot the test copy never took",
			"pvc", pvcName, "snapshot", snapshot.Name)

		if err := reclaimSourceContent(ctx, in, snapshot); err != nil {
			return err
		}
	}

	if err := in.Client.Delete(ctx, snapshot); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting source snapshot %s: %w", snapshot.Name, err)
	}

	if adopted {
		return deleteOrphanedSourceContent(ctx, in, snapshot)
	}

	return nil
}

func importedContentExists(ctx context.Context, in Input, runID, pvcName string) (bool, error) {
	content := &snapv1.VolumeSnapshotContent{}

	err := in.APIReader.Get(ctx, client.ObjectKey{Name: ImportedContentName(runID, pvcName)}, content)
	if err == nil {
		return true, nil
	}

	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	return false, fmt.Errorf("checking imported content for PVC %s: %w", pvcName, err)
}

// reclaimSourceContent undoes the Retain set during an import that never
// completed, so deleting the source snapshot also reclaims the storage.
func reclaimSourceContent(ctx context.Context, in Input, snapshot *snapv1.VolumeSnapshot) error {
	if snapshot.Status == nil || snapshot.Status.BoundVolumeSnapshotContentName == nil {
		return nil
	}

	content := &snapv1.VolumeSnapshotContent{}
	key := client.ObjectKey{Name: *snapshot.Status.BoundVolumeSnapshotContentName}

	if err := in.APIReader.Get(ctx, key, content); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("reading snapshot content %s: %w", key.Name, err)
	}

	if content.Spec.DeletionPolicy == snapv1.VolumeSnapshotContentDelete {
		return nil
	}

	patched := content.DeepCopy()
	patched.Spec.DeletionPolicy = snapv1.VolumeSnapshotContentDelete

	if err := in.Client.Patch(ctx, patched, client.MergeFrom(content)); err != nil {
		return fmt.Errorf("reclaiming snapshot content %s: %w", key.Name, err)
	}

	in.Log.Info("Reclaiming a storage snapshot the test copy never took", "content", key.Name)

	return nil
}

// deleteOrphanedSourceContent removes the source content object once its
// VolumeSnapshot is gone. Retain means only the object goes; the storage
// snapshot stays, now referenced solely by the imported content.
func deleteOrphanedSourceContent(ctx context.Context, in Input, snapshot *snapv1.VolumeSnapshot) error {
	if snapshot.Status == nil || snapshot.Status.BoundVolumeSnapshotContentName == nil {
		return nil
	}

	content := &snapv1.VolumeSnapshotContent{}
	content.Name = *snapshot.Status.BoundVolumeSnapshotContentName

	if err := in.Client.Delete(ctx, content); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting source snapshot content %s: %w", content.Name, err)
	}

	return nil
}

// deleteRequests removes the Velero requests this run created. They carry the
// run's labels rather than the VRG's owner labels, so the VRG's own kube
// object cleanup does not see them and would leave them behind.
func deleteRequests(ctx context.Context, in Input, runID string) error {
	if in.KubeObjects == nil {
		return nil
	}

	err := in.KubeObjects.RecoverRequestsDelete(ctx, in.Client, in.VeleroNamespace, RequestLabels(runID))
	if err != nil {
		return fmt.Errorf("deleting dry run restore requests: %w", err)
	}

	in.Log.Info("Deleted the dry run velero requests", "namespace", in.VeleroNamespace)

	return nil
}

// RequestLabels identify the Velero requests belonging to a run, keeping them
// disjoint from the production restore flow's.
func RequestLabels(runID string) map[string]string {
	return map[string]string{RunIDLabel: runID}
}
