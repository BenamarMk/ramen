// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"fmt"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A PVC cannot reference a VolumeSnapshot in another namespace: VolumeSnapshot
// is namespaced and dataSource is namespace-local. Cross-namespace data
// sources are still alpha and need a ReferenceGrant, so they are not an
// option.
//
// VolumeSnapshotContent, though, is cluster scoped, and its volumeSnapshotRef
// is a full ObjectReference that can point into any namespace. So the storage
// snapshot is imported into the test namespace as a pre-provisioned snapshot,
// and the test PVC is provisioned from it the ordinary way:
//
//	 workload1                  cluster-scoped          workload1-test-ID
//	 ─────────                  ──────────────          ─────────────────
//	 VolumeSnapshot             VolumeSnapshotContent
//	 db-snapshot ──── bound ──► snapcontent-abc
//	                            deletionPolicy: Retain ◄── ① patched
//	                            snapshotHandle: H
//	                                   │ ② read H, driver, volume mode
//	                                   ▼
//	                            VolumeSnapshotContent     ③ created
//	                            dryrun-<runID>-db
//	                            deletionPolicy: Delete
//	                            snapshotHandle: H
//	                            volumeSnapshotRef: ─────► VolumeSnapshot  ④
//	                                                       db-snapshot
//	                                                           │ dataSource
//	                                                           ▼
//	                                                      PVC  db         ⑤
//
// Ownership of the storage snapshot transfers with the deletion policies: the
// source side becomes Retain, giving it up, and the test side is Delete,
// taking it. Deleting the test namespace later reclaims the storage snapshot
// with it.

// ensureTestVolumes gives each protected PVC's snapshot to its test namespace
// and provisions the test PVC there. It returns the reasons it is not finished
// yet, empty when every test PVC is Bound.
func ensureTestVolumes(
	ctx context.Context, in Input, runID string, mapping map[string]string,
) ([]string, error) {
	var pending []string

	for idx := range in.VolRepPVCs {
		pvc := &in.VolRepPVCs[idx]

		target, mapped := mapping[pvc.Namespace]
		if !mapped {
			// A PVC outside the protected namespaces has no test namespace to
			// go to. Nothing to do, and nothing wrong.
			continue
		}

		reason, err := ensureTestVolume(ctx, in, runID, pvc, target)
		if err != nil {
			return nil, err
		}

		if reason != "" {
			pending = append(pending, reason)
		}
	}

	return pending, nil
}

func ensureTestVolume(
	ctx context.Context, in Input, runID string, pvc *corev1.PersistentVolumeClaim, target string,
) (string, error) {
	source, reason, err := readySourceSnapshot(ctx, in, pvc)
	if err != nil || reason != "" {
		return reason, err
	}

	content, err := releaseSourceContent(ctx, in, *source.Status.BoundVolumeSnapshotContentName)
	if err != nil {
		return "", err
	}

	if content.Status == nil || content.Status.SnapshotHandle == nil {
		return fmt.Sprintf("snapshot content %s has no storage handle yet", content.Name), nil
	}

	if err := importContent(ctx, in, runID, pvc, content, target); err != nil {
		return "", err
	}

	if err := createTestSnapshot(ctx, in, runID, pvc, target); err != nil {
		return "", err
	}

	return createTestPVC(ctx, in, runID, pvc, target)
}

// readySourceSnapshot fetches the dry-run snapshot of a protected PVC, once
// the storage has actually taken it.
func readySourceSnapshot(
	ctx context.Context, in Input, pvc *corev1.PersistentVolumeClaim,
) (*snapv1.VolumeSnapshot, string, error) {
	source := &snapv1.VolumeSnapshot{}
	key := client.ObjectKey{Name: SnapshotName(pvc.Name), Namespace: pvc.Namespace}

	if err := in.APIReader.Get(ctx, key, source); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Sprintf("waiting for snapshot %s of PVC %s", key.Name, pvc.Name), nil
		}

		return nil, "", fmt.Errorf("reading snapshot %s: %w", key.Name, err)
	}

	if source.Status == nil || source.Status.ReadyToUse == nil || !*source.Status.ReadyToUse ||
		source.Status.BoundVolumeSnapshotContentName == nil {
		return nil, fmt.Sprintf("snapshot %s is not ready to use yet", key.Name), nil
	}

	return source, "", nil
}

// releaseSourceContent makes the source side give up the storage snapshot, so
// that deleting the source VolumeSnapshot during the revert leaves the test
// namespace's copy intact.
//
// This must happen before the import below. For the window in which two
// contents reference one handle, the source has to already be the one that
// will not reclaim it, whichever is removed first.
func releaseSourceContent(
	ctx context.Context, in Input, name string,
) (*snapv1.VolumeSnapshotContent, error) {
	content := &snapv1.VolumeSnapshotContent{}
	if err := in.APIReader.Get(ctx, client.ObjectKey{Name: name}, content); err != nil {
		return nil, fmt.Errorf("reading snapshot content %s: %w", name, err)
	}

	if content.Spec.DeletionPolicy == snapv1.VolumeSnapshotContentRetain {
		return content, nil
	}

	patched := content.DeepCopy()
	patched.Spec.DeletionPolicy = snapv1.VolumeSnapshotContentRetain

	if err := in.Client.Patch(ctx, patched, client.MergeFrom(content)); err != nil {
		return nil, fmt.Errorf("retaining snapshot content %s: %w", name, err)
	}

	in.Log.Info("Released the storage snapshot to the test copy", "content", name)

	return patched, nil
}

// importContent points a new cluster-scoped content at the same storage
// snapshot, bound to a VolumeSnapshot in the test namespace. Its Delete policy
// is what makes deleting the test namespace reclaim the storage snapshot.
func importContent(
	ctx context.Context, in Input, runID string,
	pvc *corev1.PersistentVolumeClaim, source *snapv1.VolumeSnapshotContent, target string,
) error {
	imported := &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ImportedContentName(runID, pvc.Name),
			Labels: runLabels(in, runID, pvc.Namespace),
		},
		Spec: snapv1.VolumeSnapshotContentSpec{
			DeletionPolicy: snapv1.VolumeSnapshotContentDelete,
			Driver:         source.Spec.Driver,
			Source: snapv1.VolumeSnapshotContentSource{
				SnapshotHandle: source.Status.SnapshotHandle,
			},
			SourceVolumeMode:        source.Spec.SourceVolumeMode,
			VolumeSnapshotClassName: source.Spec.VolumeSnapshotClassName,
			VolumeSnapshotRef: corev1.ObjectReference{
				APIVersion: snapv1.SchemeGroupVersion.String(),
				Kind:       "VolumeSnapshot",
				Name:       SnapshotName(pvc.Name),
				Namespace:  target,
			},
		},
	}

	err := in.Client.Create(ctx, imported)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("importing snapshot content for PVC %s: %w", pvc.Name, err)
	}

	return nil
}

// createTestSnapshot is the namespaced half of the import: a pre-provisioned
// VolumeSnapshot naming the content above, which the test PVC can reference.
func createTestSnapshot(
	ctx context.Context, in Input, runID string, pvc *corev1.PersistentVolumeClaim, target string,
) error {
	name := ImportedContentName(runID, pvc.Name)

	snapshot := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SnapshotName(pvc.Name),
			Namespace: target,
			Labels:    runLabels(in, runID, pvc.Namespace),
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{VolumeSnapshotContentName: &name},
		},
	}

	err := in.Client.Create(ctx, snapshot)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating test snapshot for PVC %s: %w", pvc.Name, err)
	}

	return nil
}

// createTestPVC provisions the test volume from the imported snapshot.
//
// The PVC keeps the source PVC's name. That is load bearing: the restored Pod
// and VirtualMachine specs reference their claims by name, and renaming here
// would leave every one of them unschedulable.
func createTestPVC(
	ctx context.Context, in Input, runID string, pvc *corev1.PersistentVolumeClaim, target string,
) (string, error) {
	apiGroup := snapv1.GroupName
	testPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvc.Name,
			Namespace: target,
			Labels:    runLabels(in, runID, pvc.Namespace),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      pvc.Spec.AccessModes,
			StorageClassName: pvc.Spec.StorageClassName,
			VolumeMode:       pvc.Spec.VolumeMode,
			Resources:        corev1.VolumeResourceRequirements{Requests: pvc.Spec.Resources.Requests},
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     "VolumeSnapshot",
				Name:     SnapshotName(pvc.Name),
			},
		},
	}

	err := in.Client.Create(ctx, testPVC)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("creating test PVC %s in %s: %w", pvc.Name, target, err)
	}

	bound := &corev1.PersistentVolumeClaim{}
	if err := in.APIReader.Get(ctx,
		client.ObjectKey{Name: pvc.Name, Namespace: target}, bound); err != nil {
		return fmt.Sprintf("waiting for test PVC %s/%s", target, pvc.Name), nil //nolint:nilerr
	}

	if bound.Status.Phase != corev1.ClaimBound {
		return fmt.Sprintf("test PVC %s/%s is %s, waiting for Bound",
			target, pvc.Name, bound.Status.Phase), nil
	}

	return "", nil
}
