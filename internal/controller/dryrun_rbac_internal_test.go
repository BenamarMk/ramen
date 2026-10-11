// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"os"
	"slices"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

// config/dr-cluster/rbac/role.yaml is curated by hand -- controller-gen only
// maintains the aggregate in config/rbac -- so a kubebuilder marker gaining a
// verb does not reach the dr-cluster operator by itself. Nothing catches that
// at build time, and a fake client enforces no RBAC at all, so the first
// sighting is a forbidden error against a live cluster.
//
// Test failover has already been bitten twice: namespaces granted create but
// not delete, and configmaps the same. This checks the role actually grants
// what the dry-run paths do.
const drClusterRolePath = "../../config/dr-cluster/rbac/role.yaml"

func drClusterRole(t *testing.T) *rbacv1.ClusterRole {
	t.Helper()

	raw, err := os.ReadFile(drClusterRolePath)
	if err != nil {
		t.Fatalf("reading %s: %v", drClusterRolePath, err)
	}

	role := &rbacv1.ClusterRole{}
	if err := yaml.Unmarshal(raw, role); err != nil {
		t.Fatalf("parsing %s: %v", drClusterRolePath, err)
	}

	return role
}

func grants(role *rbacv1.ClusterRole, group, resource, verb string) bool {
	for _, rule := range role.Rules {
		if !slices.Contains(rule.APIGroups, group) || !slices.Contains(rule.Resources, resource) {
			continue
		}

		if slices.Contains(rule.Verbs, verb) || slices.Contains(rule.Verbs, "*") {
			return true
		}
	}

	return false
}

func TestDRClusterRoleGrantsWhatTheDryRunDoes(t *testing.T) {
	role := drClusterRole(t)

	for _, need := range []struct {
		group, resource, verb, why string
	}{
		{"", "namespaces", "create", "creating a test namespace"},
		{"", "namespaces", "delete", "tearing the test copy down"},
		{"", "namespaces", "get", "checking whether a test namespace is gone"},
		{"", "persistentvolumeclaims", "create", "the test copy's volumes"},
		{"", "persistentvolumeclaims", "delete", "discarding the PVCs the run restored"},
		{"", "configmaps", "create", "the ownerReferences resource modifier"},
		{"", "configmaps", "delete", "removing it again on tear down"},
		{"", "resourcequotas", "create", "copying the source namespace's posture"},
		{"", "limitranges", "create", "copying the source namespace's posture"},
		{"networking.k8s.io", "networkpolicies", "create", "copying the source namespace's posture"},
		{"rbac.authorization.k8s.io", "roles", "create", "copying the source namespace's posture"},
		{"rbac.authorization.k8s.io", "rolebindings", "create", "copying the source namespace's posture"},
		{"rbac.authorization.k8s.io", "roles", "escalate", "copying rules the operator does not hold"},
		{"rbac.authorization.k8s.io", "rolebindings", "bind", "copying rules the operator does not hold"},
		{"snapshot.storage.k8s.io", "volumesnapshotcontents", "create", "importing a storage snapshot"},
		{"snapshot.storage.k8s.io", "volumesnapshotcontents", "patch", "releasing it to the test copy"},
		{"snapshot.storage.k8s.io", "volumesnapshotcontents", "delete", "sweeping it up afterwards"},
		{"snapshot.storage.k8s.io", "volumesnapshots", "create", "the test namespace's snapshot"},
		{"snapshot.storage.k8s.io", "volumesnapshots", "delete", "handing the source snapshot over"},
	} {
		if !grants(role, need.group, need.resource, need.verb) {
			group := need.group
			if group == "" {
				group = "core"
			}

			t.Errorf("dr-cluster role does not grant %s on %s/%s, needed for %s",
				need.verb, group, need.resource, need.why)
		}
	}
}
