// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun_test

import (
	"strings"
	"testing"

	"github.com/ramendr/ramen/internal/controller/dryrun"
)

func TestRunIDIsStableAndDistinct(t *testing.T) {
	// The hub and the managed cluster derive the run ID independently from the
	// same DRPC UID and must land on the same value, so it has to be a pure
	// function of the UID.
	const uid = "0f8fad5b-d9cb-469f-a165-70867728950e"

	first := dryrun.RunID(uid)
	if second := dryrun.RunID(uid); first != second {
		t.Errorf("RunID is not stable: %q then %q", first, second)
	}

	if other := dryrun.RunID("d9b2d63d-a233-4123-847e-eb4d4a1e2eab"); other == first {
		t.Errorf("different DRPCs produced the same run ID %q", first)
	}

	if len(first) != 8 {
		t.Errorf("run ID %q is %d characters, want 8", first, len(first))
	}
}

func TestTargetNamespaceFitsTheNameLimit(t *testing.T) {
	runID := dryrun.RunID("some-uid")

	for _, source := range []string{
		"a",
		"workload1",
		strings.Repeat("n", 49),
		strings.Repeat("n", 63),
		strings.Repeat("n", 200),
	} {
		target := dryrun.TargetNamespace(source, runID)
		if len(target) > 63 {
			t.Errorf("source of %d chars produced a %d char namespace %q", len(source), len(target), target)
		}

		if !strings.HasSuffix(target, "-test-"+runID) {
			t.Errorf("namespace %q does not carry the run ID", target)
		}
	}
}

func TestNamespaceMappingCoversEveryProtectedNamespace(t *testing.T) {
	runID := dryrun.RunID("some-uid")

	mapping, err := dryrun.NamespaceMapping([]string{"workload1", "workload2"}, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mapping) != 2 {
		t.Fatalf("expected 2 mappings, got %d: %v", len(mapping), mapping)
	}

	// One shared suffix across the run, so the namespaces read as one set.
	if mapping["workload1"] != "workload1-test-"+runID || mapping["workload2"] != "workload2-test-"+runID {
		t.Errorf("unexpected mapping: %v", mapping)
	}
}

func TestNamespaceMappingRefusesTruncationCollisions(t *testing.T) {
	// Two long names sharing a prefix truncate onto one target. Merging two
	// namespaces into one would silently restore both workloads on top of
	// each other, so this has to be refused.
	long := strings.Repeat("n", 60)

	_, err := dryrun.NamespaceMapping([]string{long + "-one", long + "-two"}, dryrun.RunID("some-uid"))
	if err == nil {
		t.Fatal("expected a collision error, got none")
	}

	// The message has to name both sources: the user cannot act on it
	// otherwise, since neither name is visible in the colliding target.
	for _, want := range []string{long + "-one", long + "-two"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}
