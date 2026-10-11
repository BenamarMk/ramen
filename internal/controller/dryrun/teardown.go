// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"fmt"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TearDown deletes the test copy, once the user has finished with it.
//
// Deleting the namespace takes the restored workload, its PVCs and its
// snapshots with it, and the imported snapshot contents carry a Delete policy
// so the storage snapshots behind them are reclaimed too. Anything
// cluster-scoped that the cascade misses is swept by label afterwards.
//
// It reports whether everything is gone. Namespace deletion is not immediate
// -- finalizers on the restored workload can hold it for a while -- so the
// caller keeps asking rather than assuming one pass is enough.
func TearDown(ctx context.Context, in Input) (bool, error) {
	runID := RunID(in.DRPCUID)
	in.Log = in.Log.WithValues("runID", runID)

	status := in.VRG.Status.DryRun
	if status == nil {
		// Nothing was built here, or it has already been taken down.
		return true, nil
	}

	remaining, err := deleteTestNamespaces(ctx, in, status.NamespaceMapping)
	if err != nil {
		return false, err
	}

	if remaining > 0 {
		in.Log.Info("Waiting for the test namespaces to go away", "remaining", remaining)

		return false, nil
	}

	if err := deleteImportedContents(ctx, in, runID); err != nil {
		return false, err
	}

	in.Log.Info("Test copy deleted", "namespaces", status.NamespaceMapping)

	return true, nil
}

// deleteTestNamespaces removes the namespaces this run built and reports how
// many are still there.
func deleteTestNamespaces(ctx context.Context, in Input, mapping map[string]string) (int, error) {
	remaining := 0

	for source, target := range mapping {
		namespace := &corev1.Namespace{}

		err := in.APIReader.Get(ctx, client.ObjectKey{Name: target}, namespace)
		if k8serrors.IsNotFound(err) {
			continue
		}

		if err != nil {
			return 0, fmt.Errorf("reading test namespace %s: %w", target, err)
		}

		remaining++

		if namespace.DeletionTimestamp != nil {
			continue
		}

		// Refuse anything that is not ours. A protected namespace and its
		// test copy differ by a suffix, and deleting the wrong one takes the
		// running workload with it.
		if namespace.Labels[NamespaceOfLabel] != source {
			return 0, fmt.Errorf(
				"refusing to delete namespace %s: it is not the test copy of %s", target, source)
		}

		in.Log.Info("Deleting the test namespace", "namespace", target, "for", source)

		if err := in.Client.Delete(ctx, namespace); err != nil && !k8serrors.IsNotFound(err) {
			return 0, fmt.Errorf("deleting test namespace %s: %w", target, err)
		}
	}

	return remaining, nil
}

// deleteImportedContents sweeps up the cluster-scoped snapshot contents this
// run imported. Deleting the test namespace normally takes them, through the
// VolumeSnapshot they are bound to, but a content whose snapshot never bound
// has nothing to cascade from.
func deleteImportedContents(ctx context.Context, in Input, runID string) error {
	contents := &snapv1.VolumeSnapshotContentList{}
	if err := in.APIReader.List(ctx, contents, client.MatchingLabels(RequestLabels(runID))); err != nil {
		return fmt.Errorf("listing imported snapshot contents: %w", err)
	}

	for idx := range contents.Items {
		content := &contents.Items[idx]

		in.Log.Info("Deleting an imported snapshot content left behind", "content", content.Name)

		if err := in.Client.Delete(ctx, content); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("deleting imported snapshot content %s: %w", content.Name, err)
		}
	}

	return nil
}
