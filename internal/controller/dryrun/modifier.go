// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dryrun

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	// modifierDataKey is the key Velero reads the rules from. It insists on
	// exactly one key in the ConfigMap, which is why this run owns a
	// ConfigMap of its own rather than sharing the static-IP one.
	modifierDataKey = "resource-modifiers.yaml"

	modifierVersion = "v1"
)

// resourceModifiers mirrors the subset of Velero's resource modifier schema
// this package writes. Velero keeps the type internal, so it cannot be
// imported.
type (
	resourceModifiers struct {
		Version               string                 `json:"version"`
		ResourceModifierRules []resourceModifierRule `json:"resourceModifierRules"`
	}

	resourceModifierRule struct {
		Conditions   modifierConditions `json:"conditions"`
		MergePatches []mergePatch       `json:"mergePatches,omitempty"`
	}

	modifierConditions struct {
		GroupResource string `json:"groupResource"`
	}

	mergePatch struct {
		PatchData string `json:"patchData"`
	}
)

// modifierConfigMapName is the ConfigMap holding this run's restore rules.
func modifierConfigMapName(vrgName, runID string) string {
	return "dryrun-" + vrgName + "-" + runID + "-resource-modifier"
}

// ensureResourceModifier creates the rules Velero applies to everything it
// restores into a test namespace.
//
// There is one rule, and it strips ownerReferences. Restored objects carry the
// references recorded in the backup, which name owners in the namespace they
// came from. Those UIDs do not resolve in the test namespace, and the garbage
// collector deletes anything whose owner it cannot find -- so without this the
// test copy quietly disappears shortly after being restored.
//
// A merge patch rather than a JSON patch remove: remove fails when the path is
// absent, and most objects carry no owner references at all.
//
// The groupResource glob matches every kind, which is what Velero's own
// matching supports and what this needs -- any restored object can carry an
// owner reference.
func ensureResourceModifier(ctx context.Context, in Input, runID string) (string, error) {
	rules := resourceModifiers{
		Version: modifierVersion,
		ResourceModifierRules: []resourceModifierRule{{
			Conditions:   modifierConditions{GroupResource: "*"},
			MergePatches: []mergePatch{{PatchData: `{"metadata":{"ownerReferences":null}}`}},
		}},
	}

	encoded, err := yaml.Marshal(rules)
	if err != nil {
		return "", fmt.Errorf("encoding resource modifier rules: %w", err)
	}

	name := modifierConfigMapName(in.VRG.GetName(), runID)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: in.VeleroNamespace,
			Labels:    RequestLabels(runID),
		},
		Data: map[string]string{modifierDataKey: string(encoded)},
	}

	err = in.Client.Create(ctx, configMap)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("creating resource modifier %s: %w", name, err)
	}

	return name, nil
}
