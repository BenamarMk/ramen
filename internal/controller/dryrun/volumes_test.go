// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun_test

import (
	"context"
	"testing"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/ramendr/ramen/internal/controller/dryrun"
)

const (
	testPVCName     = "db"
	testHandle      = "0001-0011-openshift-storage-0000000000000001-abcdef"
	sourceContentNm = "snapcontent-source"
)

func ptr[T any](v T) *T { return &v }

// sourcePVC is the protected PVC a test copy is made of.
func sourcePVC() *corev1.PersistentVolumeClaim {
	mode := corev1.PersistentVolumeFilesystem

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: testPVCName, Namespace: sourceNS},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: ptr("rbd"),
			VolumeMode:       &mode,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")},
			},
		},
	}
}

// protectedVolume is that PVC plus the dry-run snapshot already taken of it
// and ready to use: the state the hand-off expects to find.
func protectedVolume(ready bool) []client.Object {
	mode := corev1.PersistentVolumeFilesystem

	return []client.Object{
		sourcePVC(),
		&snapv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dryrun.SnapshotName(testPVCName),
				Namespace: sourceNS,
				Labels:    dryrun.SnapshotSelector("test-vrg"),
			},
			Status: &snapv1.VolumeSnapshotStatus{
				ReadyToUse:                     ptr(ready),
				BoundVolumeSnapshotContentName: ptr(sourceContentNm),
			},
		},
		&snapv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: sourceContentNm},
			Spec: snapv1.VolumeSnapshotContentSpec{
				Driver:                  "openshift-storage.rbd.csi.ceph.com",
				DeletionPolicy:          snapv1.VolumeSnapshotContentDelete,
				SourceVolumeMode:        &mode,
				VolumeSnapshotClassName: ptr("rbd-snapclass"),
				VolumeSnapshotRef: corev1.ObjectReference{
					Name: dryrun.SnapshotName(testPVCName), Namespace: sourceNS,
				},
			},
			Status: &snapv1.VolumeSnapshotContentStatus{SnapshotHandle: ptr(testHandle)},
		},
	}
}

func volumeInput(t *testing.T) (dryrun.Input, client.Client) {
	t.Helper()

	return volumeInputReady(t, true)
}

func volumeInputReady(t *testing.T, ready bool) (dryrun.Input, client.Client) {
	t.Helper()

	objects := append(sourceWorld(), protectedVolume(ready)...)

	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(objects...).Build()

	in := testInput(t, []string{sourceNS})
	in.Client = c
	in.APIReader = c
	in.VRG.Name = "test-vrg"
	// The real reconcile hands over fully populated PVCs read from the
	// cluster, not name-only stubs.
	in.VolRepPVCs = []corev1.PersistentVolumeClaim{*sourcePVC()}

	return in, c
}

func TestTestVolumeIsProvisionedFromAnImportedSnapshot(t *testing.T) {
	in, c := volumeInput(t)
	ctx := context.Background()

	out, err := dryrun.Reconcile(ctx, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runID := dryrun.RunID(testDRPCUID)
	target := out.NamespaceMapping[sourceNS]

	// The import: a cluster-scoped content pointing at the same storage
	// handle, bound to a snapshot in the test namespace.
	imported := &snapv1.VolumeSnapshotContent{}
	if err := c.Get(ctx,
		client.ObjectKey{Name: dryrun.ImportedContentName(runID, testPVCName)}, imported); err != nil {
		t.Fatalf("imported content not created: %v", err)
	}

	if imported.Spec.Source.SnapshotHandle == nil || *imported.Spec.Source.SnapshotHandle != testHandle {
		t.Errorf("imported content does not reference the storage snapshot: %+v", imported.Spec.Source)
	}

	if imported.Spec.VolumeSnapshotRef.Namespace != target {
		t.Errorf("imported content points at %q, want the test namespace %q",
			imported.Spec.VolumeSnapshotRef.Namespace, target)
	}

	// Delete policy on the test side: deleting the test namespace is what
	// eventually reclaims the storage snapshot.
	if imported.Spec.DeletionPolicy != snapv1.VolumeSnapshotContentDelete {
		t.Errorf("imported content policy is %q, want Delete", imported.Spec.DeletionPolicy)
	}
}

func TestTestPVCMirrorsTheSourceClaim(t *testing.T) {
	in, c := volumeInput(t)
	ctx := context.Background()

	out, err := dryrun.Reconcile(ctx, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	target := out.NamespaceMapping[sourceNS]

	// The test PVC keeps the source name, or restored workloads would
	// reference claims that do not exist.
	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKey{Name: testPVCName, Namespace: target}, pvc); err != nil {
		t.Fatalf("test PVC not created: %v", err)
	}

	if pvc.Spec.DataSource == nil || pvc.Spec.DataSource.Name != dryrun.SnapshotName(testPVCName) {
		t.Errorf("test PVC is not provisioned from the snapshot: %+v", pvc.Spec.DataSource)
	}

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "rbd" {
		t.Errorf("test PVC storage class is %v", pvc.Spec.StorageClassName)
	}
}

func TestSourceGivesUpTheStorageSnapshotBeforeTheImport(t *testing.T) {
	// Ordering is the safety property here. While two contents reference one
	// handle, the source must already be the one that will not reclaim it,
	// whichever is removed first.
	in, c := volumeInput(t)

	if _, err := dryrun.Reconcile(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	source := &snapv1.VolumeSnapshotContent{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: sourceContentNm}, source); err != nil {
		t.Fatal(err)
	}

	if source.Spec.DeletionPolicy != snapv1.VolumeSnapshotContentRetain {
		t.Errorf("source content policy is %q, want Retain", source.Spec.DeletionPolicy)
	}
}

func TestBuildWaitsForAnUnboundTestPVC(t *testing.T) {
	// The kube objects must not be restored against a claim that never binds:
	// the workload would come up Pending with nothing to explain why.
	in, _ := volumeInput(t)

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	if out.State != "InProgress" {
		t.Errorf("state %q, want InProgress while the PVC is unbound", out.State)
	}

	if out.Message == "" {
		t.Error("no reason given for waiting")
	}
}

func TestBuildWaitsForASnapshotThatIsNotReady(t *testing.T) {
	in, c := volumeInputReady(t, false)

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	if out.State != "InProgress" {
		t.Errorf("state %q, want InProgress", out.State)
	}

	// Nothing may be imported from a snapshot the storage has not finished.
	imported := &snapv1.VolumeSnapshotContent{}

	err = c.Get(context.Background(),
		client.ObjectKey{Name: dryrun.ImportedContentName(dryrun.RunID(testDRPCUID), testPVCName)}, imported)
	if err == nil {
		t.Error("imported a snapshot that was not ready to use")
	}
}

func TestHandOffLeavesAnAdoptedSnapshotToTheTestCopy(t *testing.T) {
	in, c := volumeInput(t)
	ctx := context.Background()

	// Build the test copy, so the import exists and owns the storage.
	if _, err := dryrun.Reconcile(ctx, in); err != nil {
		t.Fatal(err)
	}

	if err := dryrun.HandOff(ctx, in); err != nil {
		t.Fatalf("hand-off failed: %v", err)
	}

	// The source side is gone...
	snapshot := &snapv1.VolumeSnapshot{}

	err := c.Get(ctx, client.ObjectKey{Name: dryrun.SnapshotName(testPVCName), Namespace: sourceNS}, snapshot)
	if err == nil {
		t.Error("source snapshot survived the hand-off")
	}

	if err := c.Get(ctx, client.ObjectKey{Name: sourceContentNm}, &snapv1.VolumeSnapshotContent{}); err == nil {
		t.Error("source snapshot content survived the hand-off")
	}

	// ...and the test side still holds the storage.
	imported := &snapv1.VolumeSnapshotContent{}
	if err := c.Get(ctx,
		client.ObjectKey{Name: dryrun.ImportedContentName(dryrun.RunID(testDRPCUID), testPVCName)},
		imported); err != nil {
		t.Fatalf("imported content was destroyed: %v", err)
	}

	if *imported.Spec.Source.SnapshotHandle != testHandle {
		t.Error("imported content lost its storage handle")
	}
}

func TestHandOffReclaimsASnapshotNothingAdopted(t *testing.T) {
	// A run aborted before this volume was handed over must not leak the
	// storage snapshot: with no imported content referencing the handle,
	// nothing would ever reclaim it.
	in, c := volumeInput(t)
	ctx := context.Background()

	if err := dryrun.HandOff(ctx, in); err != nil {
		t.Fatalf("hand-off failed: %v", err)
	}

	content := &snapv1.VolumeSnapshotContent{}
	if err := c.Get(ctx, client.ObjectKey{Name: sourceContentNm}, content); err != nil {
		t.Fatalf("source content should still exist to reclaim the storage: %v", err)
	}

	if content.Spec.DeletionPolicy != snapv1.VolumeSnapshotContentDelete {
		t.Errorf("source content policy is %q, want Delete so the storage is reclaimed",
			content.Spec.DeletionPolicy)
	}
}

func TestHandOffToleratesHavingNothingToDo(t *testing.T) {
	// It runs on every path that ends a run, including ones where no snapshot
	// was ever taken.
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()

	in := testInput(t, []string{sourceNS})
	in.Client = c
	in.APIReader = c
	in.VRG.Name = "test-vrg"

	if err := dryrun.HandOff(context.Background(), in); err != nil {
		t.Errorf("hand-off failed with nothing to hand off: %v", err)
	}
}
