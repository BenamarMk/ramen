// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"os"
	"strings"
	"testing"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	rmn "github.com/ramendr/ramen/api/v1alpha1"
)

// The dryRun printer column reads the annotation rather than a status field,
// so its JSONPath has to escape the dots in the annotation key. That escaping
// survives a round trip through a kubebuilder marker only because the marker
// doubles the backslashes, and nothing about it is checked at build time.
//
// Getting it wrong fails silently: the API server accepts any syntactically
// valid path, and a path that matches nothing renders an empty cell for every
// DRPC. The column would simply stop working, with no error anywhere.
//
// Verified against a real API server at the time it was added -- an annotated
// DRPC rendered SettingUp, an un-annotated one rendered an empty cell. This
// pins the generated path so a reformatted marker cannot quietly undo it.
const drpcCRDPath = "../../config/crd/bases/ramendr.openshift.io_drplacementcontrols.yaml"

func dryRunColumn(columns []apiextensions.CustomResourceColumnDefinition,
) *apiextensions.CustomResourceColumnDefinition {
	for i := range columns {
		if columns[i].Name == "dryRun" {
			return &columns[i]
		}
	}

	return nil
}

func TestDRPCDryRunPrinterColumn(t *testing.T) {
	raw, err := os.ReadFile(drpcCRDPath)
	if err != nil {
		t.Fatalf("reading %s: %v", drpcCRDPath, err)
	}

	crd := &apiextensions.CustomResourceDefinition{}
	if err := yaml.Unmarshal(raw, crd); err != nil {
		t.Fatalf("parsing %s: %v", drpcCRDPath, err)
	}

	// Escaped dots, unescaped slash -- the form the API server's JSONPath
	// parser resolves against an annotation key.
	const want = `.metadata.annotations.drplacementcontrol\.ramendr\.openshift\.io/dry-run`

	// Unescaping `want` must land back on the real annotation key, so renaming
	// the constant breaks this loudly instead of leaving the column pointed at
	// a key that no longer exists.
	if unescaped := strings.ReplaceAll(want, `\.`, `.`); unescaped != `.metadata.annotations.`+rmn.DryRunAnnotation {
		t.Fatalf("printer column no longer tracks DryRunAnnotation:\n column %s\n   want .metadata.annotations.%s",
			unescaped, rmn.DryRunAnnotation)
	}

	for _, version := range crd.Spec.Versions {
		column := dryRunColumn(version.AdditionalPrinterColumns)
		if column == nil {
			t.Errorf("version %s has no dryRun printer column", version.Name)

			continue
		}

		if column.JSONPath != want {
			t.Errorf("version %s dryRun column JSONPath:\n got %s\nwant %s",
				version.Name, column.JSONPath, want)
		}

		// Priority 2 keeps it out of the default `kubectl get drpc`, so the
		// column every non-testing user never fills stays hidden unless they
		// ask for -o wide.
		if column.Priority != 2 {
			t.Errorf("version %s dryRun column priority = %d, want 2 (wide only)",
				version.Name, column.Priority)
		}
	}
}
