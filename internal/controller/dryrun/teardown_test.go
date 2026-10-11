// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun_test

import (
	"context"
	"testing"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
)

// builtCopy is the state after a run finished: a test namespace labeled as
// the copy of a protected one, and the imported content behind its volume.
func builtCopy(t *testing.T) (dryrun.Input, client.Client, string) {
	t.Helper()

	runID := dryrun.RunID(testDRPCUID)
	target := dryrun.TargetNamespace(sourceNS, runID)

	objects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNS}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: target,
			Labels: map[string]string{
				dryrun.NamespaceOfLabel: sourceNS,
				dryrun.RunIDLabel:       runID,
			},
		}},
		&snapv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{
			Name:   dryrun.ImportedContentName(runID, testPVCName),
			Labels: map[string]string{dryrun.RunIDLabel: runID},
		}},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(objects...).Build()

	in := testInput(t, []string{sourceNS})
	in.Client = c
	in.APIReader = c
	in.VRG.Status.DryRun = &rmn.DryRunStatus{
		RunID:            runID,
		NamespaceMapping: map[string]string{sourceNS: target},
	}

	return in, c, target
}

// tearDownUntilDone drives tear down the way the controller does: namespace
// deletion is asynchronous, so the first pass only asks and later ones check.
func tearDownUntilDone(t *testing.T, in dryrun.Input) {
	t.Helper()

	for attempt := range 5 {
		done, err := dryrun.TearDown(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error on pass %d: %v", attempt, err)
		}

		if done {
			return
		}
	}

	t.Fatal("tear down never finished")
}

func TestTearDownDeletesTheTestCopy(t *testing.T) {
	in, c, target := builtCopy(t)
	ctx := context.Background()

	tearDownUntilDone(t, in)

	if err := c.Get(ctx, client.ObjectKey{Name: target}, &corev1.Namespace{}); err == nil {
		t.Error("the test namespace survived tear down")
	}

	// The protected namespace is not the test copy and must be untouched.
	if err := c.Get(ctx, client.ObjectKey{Name: sourceNS}, &corev1.Namespace{}); err != nil {
		t.Errorf("tear down deleted the protected namespace: %v", err)
	}
}

func TestTearDownSweepsTheImportedContents(t *testing.T) {
	// Deleting the namespace normally takes these through the VolumeSnapshot
	// they are bound to, but a content whose snapshot never bound has
	// nothing to cascade from, and it holds a storage snapshot.
	in, c, _ := builtCopy(t)
	ctx := context.Background()

	tearDownUntilDone(t, in)

	name := dryrun.ImportedContentName(dryrun.RunID(testDRPCUID), testPVCName)
	if err := c.Get(ctx, client.ObjectKey{Name: name}, &snapv1.VolumeSnapshotContent{}); err == nil {
		t.Error("an imported snapshot content survived tear down, holding its storage snapshot")
	}
}

func TestTearDownRefusesANamespaceThatIsNotTheCopy(t *testing.T) {
	// A protected namespace and its test copy differ by a suffix. Deleting
	// the wrong one takes the running workload with it, so a namespace that
	// does not claim to be the copy of what it is mapped from is refused.
	in, c, target := builtCopy(t)
	ctx := context.Background()

	namespace := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: target}, namespace); err != nil {
		t.Fatal(err)
	}

	namespace.Labels[dryrun.NamespaceOfLabel] = "some-other-namespace"
	if err := c.Update(ctx, namespace); err != nil {
		t.Fatal(err)
	}

	if _, err := dryrun.TearDown(ctx, in); err == nil {
		t.Fatal("deleted a namespace that does not claim to be this copy")
	}

	if err := c.Get(ctx, client.ObjectKey{Name: target}, &corev1.Namespace{}); err != nil {
		t.Error("the namespace was deleted despite the refusal")
	}
}

func TestTearDownWaitsWhileTheNamespaceIsStillGoing(t *testing.T) {
	// Deleting a namespace is not immediate -- finalizers on the restored
	// workload can hold it -- so one pass is never enough to declare the
	// copy gone.
	in, _, _ := builtCopy(t)

	done, err := dryrun.TearDown(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if done {
		t.Error("reported the copy gone in the pass that only asked for it")
	}
}

func TestTearDownIsDoneWhenThereIsNothingToDelete(t *testing.T) {
	in, _, _ := builtCopy(t)
	in.VRG.Status.DryRun = nil

	done, err := dryrun.TearDown(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !done {
		t.Error("tear down did not finish with no copy recorded")
	}
}
