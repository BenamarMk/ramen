// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"fmt"
	"strings"
)

// Eligible reports why this workload cannot have a test copy built, or "" if
// it can.
//
// The hub refuses what it can see -- VolSync, Sync DR, non-discovered apps --
// before it ever promotes a cluster. What is left here is the one thing only
// this side can judge, because only this side can resolve a StorageClass to a
// provisioner.
//
// A refusal stalls the run rather than failing it. The reason reaches the user
// through DRPC status, and clearing the flag aborts; meanwhile the evidence
// stays put. That is the same treatment every other blocked condition gets,
// and it means the caller has exactly one shape of bad news to handle.
func Eligible(in Input) string {
	if len(in.CephFSPVCs) > 0 {
		in.Log.Info("Refusing a CephFS workload; a partial test copy is worse than none",
			"pvcs", in.CephFSPVCs)

		// A partial copy -- some volumes present, some silently missing --
		// is worse than none: the pods referencing the absent PVCs would sit
		// Pending with nothing to explain why. CephFS volumes are skipped by
		// the dry-run snapshot path, so a workload using them could only ever
		// get a partial copy.
		return fmt.Sprintf(
			"test failover does not yet support CephFS volumes; PVCs %s are CephFS backed",
			strings.Join(in.CephFSPVCs, ", "))
	}

	if len(in.VolRepPVCs) == 0 {
		return "no protected PVCs to build a test copy from"
	}

	return ""
}
