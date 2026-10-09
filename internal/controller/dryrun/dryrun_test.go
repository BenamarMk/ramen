// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun_test

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/internal/controller/dryrun"
)

const testDRPCUID = "0f8fad5b-d9cb-469f-a165-70867728950e"

// testInput is a workload ready to have a test copy built: the protected
// namespaces exist, and the client is empty of anything else.
func testInput(t *testing.T, protected []string, pvcs ...string) dryrun.Input {
	t.Helper()

	claims := make([]corev1.PersistentVolumeClaim, 0, len(pvcs))
	for _, name := range pvcs {
		claims = append(claims, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		})
	}

	seed := make([]client.Object, 0, len(protected))
	for _, ns := range protected {
		seed = append(seed, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	}

	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(seed...).Build()

	return dryrun.Input{
		Client:    c,
		APIReader: c,
		Log:       logr.Discard(),
		DRPCUID:   testDRPCUID,
		VRG: &rmn.VolumeReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ramen-ops"},
			Spec:       rmn.VolumeReplicationGroupSpec{ProtectedNamespaces: &protected},
		},
		VolRepPVCs: claims,
	}
}

func TestEligibleAcceptsAnRBDWorkload(t *testing.T) {
	if reason := dryrun.Eligible(testInput(t, []string{"workload1"}, "db")); reason != "" {
		t.Errorf("refused an eligible workload: %s", reason)
	}
}

func TestEligibleRefusesCephFS(t *testing.T) {
	// CephFS volumes are skipped by the dry-run snapshot path, so a workload
	// using them could only get a partial copy: pods referencing the absent
	// PVCs would sit Pending with nothing to explain why.
	in := testInput(t, []string{"workload1"}, "db", "shared")
	in.CephFSPVCs = []string{"shared"}

	reason := dryrun.Eligible(in)
	if reason == "" {
		t.Fatal("expected CephFS to be refused")
	}

	if !strings.Contains(reason, "shared") {
		t.Errorf("reason does not name the offending PVC: %s", reason)
	}
}

func TestEligibleRefusesAWorkloadWithNoVolumes(t *testing.T) {
	if reason := dryrun.Eligible(testInput(t, []string{"workload1"})); reason == "" {
		t.Error("expected a workload with no protected PVCs to be refused")
	}
}

func TestReconcileReportsTheNamespacesItWillBuild(t *testing.T) {
	in := testInput(t, []string{"workload1", "workload2"}, "db")

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The mapping is reported even before anything is built, because it is
	// how a user finds the test copy and how the hub mirrors it.
	runID := dryrun.RunID(testDRPCUID)
	for _, source := range []string{"workload1", "workload2"} {
		if got := out.NamespaceMapping[source]; got != source+"-test-"+runID {
			t.Errorf("mapping for %q is %q", source, got)
		}
	}

	if out.RunID != runID {
		t.Errorf("run ID %q, want %q", out.RunID, runID)
	}
}

func TestReconcileStallsRatherThanFailingOnAnIneligibleWorkload(t *testing.T) {
	// There is no failure state. A refusal has to come back as a reason the
	// hub can surface, not an error, so the run keeps its evidence until a
	// human aborts it.
	in := testInput(t, []string{"workload1"}, "shared")
	in.CephFSPVCs = []string{"shared"}

	out, err := dryrun.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatalf("ineligibility must not surface as an error: %v", err)
	}

	if out.State != rmn.DryRunBuildInProgress {
		t.Errorf("state %q, want InProgress", out.State)
	}

	if out.Message == "" {
		t.Error("a stalled run must say why")
	}

	if !out.Requeue {
		t.Error("a stalled run must ask to be called again")
	}
}

func TestReconcileNeverReportsCompletedYet(t *testing.T) {
	// Phases 4 to 6 build the copy. Until they land nothing may claim the
	// copy is ready, or the hub would revert over a half-built test.
	out, err := dryrun.Reconcile(context.Background(), testInput(t, []string{"workload1"}, "db"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.State == rmn.DryRunBuildCompleted {
		t.Error("reported Completed with nothing built")
	}
}
