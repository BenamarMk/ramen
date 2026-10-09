// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"testing"
	"time"

	ramen "github.com/ramendr/ramen/api/v1alpha1"

	"github.com/ramendr/ramen/e2e/config"
	"github.com/ramendr/ramen/e2e/deployers"
	"github.com/ramendr/ramen/e2e/dractions"
	"github.com/ramendr/ramen/e2e/test"
	"github.com/ramendr/ramen/e2e/types"
	"github.com/ramendr/ramen/e2e/util"
	"github.com/ramendr/ramen/e2e/validate"
	"github.com/ramendr/ramen/e2e/workloads"
)

// TestDryRun exercises test failover: a copy of the workload built beside it,
// in namespaces Ramen generates, with the cluster it borrowed handed back
// afterwards and the copy left running.
//
// The assertions that matter most are about what does not happen. A test
// failover that moves the workload, changes spec.action, or leaves the peer
// Primary has stopped being a test.
//
// Scope for this release is discovered applications on VolRep with RBD
// volumes; everything else is refused up front, so the test skips rather than
// fails when the configured workload does not qualify.
func TestDryRun(dt *testing.T) {
	t := test.WithLog(dt, Ctx.Logger())
	t.Parallel()

	if len(Ctx.Config().Tests) == 0 {
		t.Fatal("No tests found in the configuration file")
	}

	if err := validate.TestConfig(Ctx); err != nil {
		t.Fatal(err.Error())
	}

	if err := util.EnsureChannel(Ctx); err != nil {
		t.Fatalf("Failed to ensure channel: %s", err)
	}

	t.Cleanup(func() {
		timedCtx, cancel := Ctx.WithTimeout(1 * time.Minute)
		defer cancel()

		if err := util.EnsureChannelDeleted(timedCtx); err != nil {
			t.Fatalf("Failed to ensure channel deleted: %s", err)
		}
	})

	for _, ctx := range dryRunContexts(t) {
		t.Run(ctx.Name(), func(dt *testing.T) {
			t := test.WithLog(dt, ctx.Logger())
			t.Parallel()
			runDryRunFlow(t, ctx)
		})
	}
}

func dryRunContexts(t *test.T) []*test.Context {
	t.Helper()

	pvcSpecs := config.PVCSpecsMap(Ctx.Config())
	deploySpecs := config.DeployersMap(Ctx.Config())

	contexts := make([]*test.Context, 0, len(Ctx.Config().Tests))

	for _, tc := range Ctx.Config().Tests {
		pvcSpec, ok := pvcSpecs[tc.PVCSpec]
		if !ok {
			t.Fatalf("unknown pvcSpec %q", tc.PVCSpec)
		}

		workload, err := workloads.New(tc.Workload, Ctx.Config().Repo.Branch, pvcSpec)
		if err != nil {
			t.Fatalf("building workload: %s", err)
		}

		deployerSpec, ok := deploySpecs[tc.Deployer]
		if !ok {
			t.Fatalf("unknown deployer %q", tc.Deployer)
		}

		deployer, err := deployers.New(deployerSpec)
		if err != nil {
			t.Fatalf("building deployer: %s", err)
		}

		context := test.NewContext(Ctx, tc.ContextName(), workload, deployer)
		contexts = append(contexts, &context)
	}

	return contexts
}

func runDryRunFlow(t *test.T, ctx *test.Context) {
	t.Helper()

	if err := ctx.Validate(); err != nil {
		t.Skipf("Skip test: %s", err)
	}

	// A test failover is refused for anything but a discovered application:
	// the scope for this release is the VM and discovered-app path.
	if !ctx.Deployer().IsDiscovered() {
		t.Skip("Skip test: test failover supports discovered applications only")
	}

	if !t.Run("Deploy", ctx.Deploy) {
		t.FailNow()
	}

	if !t.Run("Enable", ctx.Enable) {
		t.FailNow()
	}

	runDryRunSteps(t, ctx)

	if !t.Run("Disable", ctx.Disable) {
		t.FailNow()
	}

	if !t.Run("Undeploy", ctx.Undeploy) {
		t.FailNow()
	}
}

// runDryRunSteps is the test failover itself: build the copy, check it landed
// and that the workload never noticed, then take the copy down.
func runDryRunSteps(t *test.T, ctx *test.Context) {
	t.Helper()

	var testNamespaces map[string]string

	if !t.Run("DryRun", func(dt *testing.T) {
		t := test.WithLog(dt, ctx.Logger())

		namespaces, err := dractions.DryRun(ctx)
		if err != nil {
			t.Fatalf("Test failover failed: %s", err)
		}

		testNamespaces = namespaces
	}) {
		t.FailNow()
	}

	if !t.Run("ValidateTestCopy", func(dt *testing.T) {
		validateTestCopy(test.WithLog(dt, ctx.Logger()), ctx, testNamespaces)
	}) {
		t.FailNow()
	}

	if !t.Run("ValidateWorkloadUntouched", func(dt *testing.T) {
		validateWorkloadUntouched(test.WithLog(dt, ctx.Logger()), ctx)
	}) {
		t.FailNow()
	}

	if !t.Run("RefuseRepeatRun", func(dt *testing.T) {
		t := test.WithLog(dt, ctx.Logger())

		if err := dractions.DryRunRefusedWhileNotCleanedUp(ctx); err != nil {
			t.Fatalf("Repeat test failover was not refused: %s", err)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("CleanupTestCopy", func(dt *testing.T) {
		t := test.WithLog(dt, ctx.Logger())

		if err := dractions.CleanupDryRun(ctx, testNamespaces); err != nil {
			t.Fatalf("Failed to clean up the test copy: %s", err)
		}
	}) {
		t.FailNow()
	}
}

// validateTestCopy checks the copy landed where it was supposed to: one test
// namespace per protected namespace, each holding the PVCs of the namespace
// it mirrors.
func validateTestCopy(t *test.T, ctx *test.Context, namespaces map[string]string) {
	t.Helper()

	if len(namespaces) == 0 {
		t.Fatal("No test namespaces reported")
	}

	cluster, err := dryRunTestCluster(ctx)
	if err != nil {
		t.Fatalf("Failed to find the test cluster: %s", err)
	}

	for source, target := range namespaces {
		t.Logf("Validating test namespace %q for protected namespace %q", target, source)

		if err := dractions.WaitForTestNamespace(ctx, cluster, target); err != nil {
			t.Fatalf("Test namespace %q was not created on cluster %q: %s", target, cluster.Name, err)
		}
	}
}

// validateWorkloadUntouched is the real assertion of this test. Everything
// about the running workload has to be exactly as it was: a test failover
// that disturbs it has failed even if the copy came up perfectly.
func validateWorkloadUntouched(t *test.T, ctx *test.Context) {
	t.Helper()

	cluster, err := util.GetCurrentCluster(ctx, ctx.ManagementNamespace(), ctx.Name())
	if err != nil {
		t.Fatalf("Failed to find the cluster running the workload: %s", err)
	}

	// The workload never moved, so it must still be healthy where it always
	// was, and its volumes must still be replicating.
	if err := deployers.WaitWorkloadHealth(ctx, cluster); err != nil {
		t.Fatalf("Workload is not healthy on %q after a test failover: %s", cluster.Name, err)
	}

	peer, err := dryRunTestCluster(ctx)
	if err != nil {
		t.Fatalf("Failed to find the test cluster: %s", err)
	}

	// The borrowed cluster must be back to Secondary, or replication is still
	// stopped and the RPO clock is still running.
	if err := dractions.ValidateVRGState(ctx, peer, ramen.Secondary, ramen.SecondaryState); err != nil {
		t.Fatalf("Test cluster %q was not handed back: %s", peer.Name, err)
	}

	if err := dractions.ValidateVRGState(ctx, cluster, ramen.Primary, ramen.PrimaryState); err != nil {
		t.Fatalf("Workload cluster %q is no longer Primary: %s", cluster.Name, err)
	}
}

func dryRunTestCluster(ctx *test.Context) (*types.Cluster, error) {
	current, err := util.GetCurrentCluster(ctx, ctx.ManagementNamespace(), ctx.Name())
	if err != nil {
		return nil, err
	}

	return dractions.TargetCluster(ctx, current.Name)
}
