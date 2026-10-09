// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

// Package dryrun builds a test copy of a protected application beside the real
// workload, in namespaces Ramen generates, so that a test failover can be
// reverted while the copy keeps running.
package dryrun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	// runIDLength is how much of the hash is kept. Eight hex characters tell
	// DRPCs apart while leaving most of the namespace budget to the source
	// name.
	runIDLength = 8

	// namespaceInfix separates a source namespace from the run ID.
	namespaceInfix = "-test-"

	// maxNamespaceName is the Kubernetes limit on a namespace name.
	maxNamespaceName = 63
)

// RunID identifies a test failover.
//
// It is derived from the DRPC UID rather than generated, so the hub and the
// managed cluster arrive at the same value independently: the hub holds the
// UID and the managed cluster receives it as an annotation on the VRG. Nothing
// has to be carried between them, and nothing is lost if status is.
//
// The consequence is that every run on a given DRPC targets the same test
// namespaces. Ramen does not detect a previous run's leftovers -- it creates
// what is missing and adopts what is already there -- so a repeat run over
// namespaces the user never deleted will reuse the earlier copy.
func RunID(drpcUID string) string {
	sum := sha256.Sum256([]byte(drpcUID))

	return hex.EncodeToString(sum[:])[:runIDLength]
}

// TargetNamespace returns the test namespace that sourceNamespace's contents
// are restored into. Namespace names are capped, so a long source name is
// truncated to make room for the suffix.
func TargetNamespace(sourceNamespace, runID string) string {
	suffix := namespaceInfix + runID

	if budget := maxNamespaceName - len(suffix); len(sourceNamespace) > budget {
		sourceNamespace = sourceNamespace[:budget]
	}

	return sourceNamespace + suffix
}

// NamespaceMapping maps every protected namespace to its test namespace. It is
// the mapping handed to Velero, and the mapping reported to the user.
//
// Truncation can make two long source names collide on a single target. That
// must be refused rather than silently merging two namespaces into one, so the
// error names both sources.
func NamespaceMapping(sourceNamespaces []string, runID string) (map[string]string, error) {
	mapping := make(map[string]string, len(sourceNamespaces))
	claimedBy := make(map[string]string, len(sourceNamespaces))

	for _, source := range sourceNamespaces {
		target := TargetNamespace(source, runID)

		if other, taken := claimedBy[target]; taken {
			return nil, fmt.Errorf(
				"protected namespaces %q and %q both map to test namespace %q: "+
					"their names are too long to tell apart once truncated",
				other, source, target)
		}

		claimedBy[target] = source
		mapping[source] = target
	}

	return mapping, nil
}

const (
	// SnapshotLabel marks a snapshot taken for a test failover.
	SnapshotLabel = "ramendr.openshift.io/dry-run-snapshot"

	// VRGLabel identifies which VRG a dry-run snapshot belongs to.
	VRGLabel = "ramendr.openshift.io/dry-run-vrg"
)

// SnapshotName is the dry-run snapshot of a protected PVC.
//
// Two sides depend on this name: the VRG takes the snapshot while the volumes
// are promoted, and the hand-off looks it back up to give it to the test
// namespace. It is defined here so neither side can drift from the other.
func SnapshotName(pvcName string) string {
	return pvcName + "-snapshot"
}

// SnapshotSelector matches every dry-run snapshot belonging to a VRG.
func SnapshotSelector(vrgName string) map[string]string {
	return map[string]string{
		SnapshotLabel: "true",
		VRGLabel:      vrgName,
	}
}

// ImportedContentName is the cluster-scoped VolumeSnapshotContent that hands a
// storage snapshot to a test namespace. Deterministic, so the hand-off can
// find it again without a label search.
func ImportedContentName(runID, pvcName string) string {
	return "dryrun-" + runID + "-" + pvcName
}
