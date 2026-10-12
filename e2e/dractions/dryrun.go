// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package dractions

import (
	"fmt"
	"time"

	ramen "github.com/ramendr/ramen/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/ramendr/ramen/e2e/types"
	"github.com/ramendr/ramen/e2e/util"
)

// A test failover builds a copy of the workload beside it, in namespaces
// Ramen generates, and then puts the cluster it borrowed back to Secondary.
// What makes it a test rather than a failover is as much what does not happen
// as what does: the workload never moves, its Placement never changes, and
// spec.action is left exactly as it was.
//
// These helpers drive and check that, so the e2e can assert the absence as
// well as the presence.

// DryRun runs a full test failover and leaves the test copy running.
//
// It sets spec.dryRun, waits for the cycle to reach its terminal phase, and
// returns the namespaces the copy was built in. Ramen reverts on its own, so
// nothing here asks it to.
func DryRun(ctx types.TestContext) (map[string]string, error) {
	log := ctx.Logger()
	name := ctx.Name()
	managementNamespace := ctx.ManagementNamespace()

	before, err := getDRPC(ctx, managementNamespace, name)
	if err != nil {
		return nil, err
	}

	log.Infof("Starting test failover for workload %q", name)

	if err := setDryRun(ctx, managementNamespace, name, true); err != nil {
		return nil, err
	}

	if err := waitDryRunPhase(ctx, managementNamespace, name, ramen.DryRunReady); err != nil {
		return nil, err
	}

	after, err := getDRPC(ctx, managementNamespace, name)
	if err != nil {
		return nil, err
	}

	if err := validateDryRunLeftTheWorkloadAlone(before, after); err != nil {
		return nil, err
	}

	if after.Status.DryRun == nil || len(after.Status.DryRun.NamespaceMapping) == 0 {
		return nil, fmt.Errorf("drpc %q reported no test namespaces", name)
	}

	log.Infof("Test failover complete, test copy running in %v", after.Status.DryRun.NamespaceMapping)

	return after.Status.DryRun.NamespaceMapping, nil
}

// validateDryRunLeftTheWorkloadAlone is the point of the whole feature: a
// test must be invisible to the real workload. If any of this drifts, the
// test failover has become a real one.
func validateDryRunLeftTheWorkloadAlone(before, after *ramen.DRPlacementControl) error {
	if before.Spec.Action != after.Spec.Action {
		return fmt.Errorf("test failover changed spec.action from %q to %q",
			before.Spec.Action, after.Spec.Action)
	}

	if before.Status.Phase != after.Status.Phase {
		return fmt.Errorf("test failover changed the DR phase from %q to %q",
			before.Status.Phase, after.Status.Phase)
	}

	beforeCluster := before.GetAnnotations()[lastAppDeploymentCluster]
	if afterCluster := after.GetAnnotations()[lastAppDeploymentCluster]; beforeCluster != afterCluster {
		return fmt.Errorf("test failover moved the workload from %q to %q", beforeCluster, afterCluster)
	}

	return nil
}

// lastAppDeploymentCluster records where the workload actually runs. A test
// failover must never touch it.
const lastAppDeploymentCluster = "drplacementcontrol.ramendr.openshift.io/last-app-deployment-cluster"

// CleanupDryRun asks Ramen to take the test copy down, by clearing the flag
// that kept it up, and waits until it is gone.
//
// Clearing spec.dryRun is the whole gesture: Ramen deletes the test
// namespaces and then removes its own annotation, which is what frees the
// DRPC to run another test.
func CleanupDryRun(ctx types.TestContext, namespaces map[string]string) error {
	log := ctx.Logger()
	managementNamespace := ctx.ManagementNamespace()
	name := ctx.Name()

	log.Infof("Tearing down the test copy in %v", namespaces)

	if err := setDryRun(ctx, managementNamespace, name, false); err != nil {
		return err
	}

	if err := waitDryRunGone(ctx, managementNamespace, name); err != nil {
		return err
	}

	cluster, err := dryRunTargetCluster(ctx)
	if err != nil {
		return err
	}

	// Ramen says it is done when the annotation goes. Check the namespaces
	// really went with it rather than taking that on trust.
	for _, testNamespace := range namespaces {
		err := cluster.Client.Get(ctx.Context(),
			k8stypes.NamespacedName{Name: testNamespace}, &corev1.Namespace{})
		if err == nil {
			return fmt.Errorf("test namespace %q survived tear down on cluster %q",
				testNamespace, cluster.Name)
		}

		if !k8serrors.IsNotFound(err) {
			return err
		}
	}

	log.Info("Test copy torn down")

	return nil
}

// waitDryRunGone waits for Ramen to remove the dry-run annotation, which it
// does only once the test copy has actually been deleted.
func waitDryRunGone(ctx types.TestContext, namespace, name string) error {
	log := ctx.Logger()
	start := time.Now()

	for {
		drpc, err := getDRPC(ctx, namespace, name)
		if err != nil {
			return err
		}

		phase := ramen.DryRunPhase(drpc.GetAnnotations()[ramen.DryRunAnnotation])
		if phase == "" {
			log.Debugf("drpc \"%s/%s\" finished tearing down in %.3f seconds",
				namespace, name, time.Since(start).Seconds())

			return nil
		}

		if err := util.Sleep(ctx.Context(), util.RetryInterval); err != nil {
			return fmt.Errorf("drpc %q is still at dry run phase %q (%s): %w",
				name, phase, dryRunMessage(drpc), err)
		}
	}
}

// dryRunTargetCluster is the cluster the test copy was built on: the one not
// running the workload.
func dryRunTargetCluster(ctx types.TestContext) (*types.Cluster, error) {
	current, err := util.GetCurrentCluster(ctx, ctx.ManagementNamespace(), ctx.Name())
	if err != nil {
		return nil, err
	}

	return getTargetCluster(ctx, ctx.Env().Hub, ctx.Config().DRPolicy, current.Name)
}

func setDryRun(ctx types.TestContext, namespace, name string, dryRun bool) error {
	drpc, err := getDRPC(ctx, namespace, name)
	if err != nil {
		return err
	}

	drpc.Spec.DryRun = dryRun

	return updateDRPC(ctx, drpc)
}

func waitDryRunPhase(ctx types.TestContext, namespace, name string, phase ramen.DryRunPhase) error {
	log := ctx.Logger()
	start := time.Now()

	log.Debugf("Waiting until drpc \"%s/%s\" reaches dry run phase %q", namespace, name, phase)

	for {
		drpc, err := getDRPC(ctx, namespace, name)
		if err != nil {
			return err
		}

		current := ramen.DryRunPhase(drpc.GetAnnotations()[ramen.DryRunAnnotation])
		if current == phase {
			log.Debugf("drpc \"%s/%s\" reached dry run phase %q in %.3f seconds",
				namespace, name, phase, time.Since(start).Seconds())

			return nil
		}

		// A test failover has no timeout by design: a run that cannot finish
		// waits for a human. The reason it is waiting is the useful thing to
		// report when the test eventually gives up.
		if err := util.Sleep(ctx.Context(), util.RetryInterval); err != nil {
			return fmt.Errorf("drpc %q dry run phase is %q, not %q (%s): %w",
				name, current, phase, dryRunMessage(drpc), err)
		}
	}
}

func dryRunMessage(drpc *ramen.DRPlacementControl) string {
	if drpc.Status.DryRun == nil || drpc.Status.DryRun.Message == "" {
		return "no reason reported"
	}

	return drpc.Status.DryRun.Message
}

// TargetCluster is the peer of the named cluster in the DR policy: during a
// test failover, the cluster the copy is built on.
func TargetCluster(ctx types.TestContext, currentCluster string) (*types.Cluster, error) {
	return getTargetCluster(ctx, ctx.Env().Hub, ctx.Config().DRPolicy, currentCluster)
}

// ValidateVRGState checks a cluster's VRG settled where a test failover
// should have left it. The borrowed cluster must be back to Secondary --
// otherwise replication is still stopped -- and the one running the workload
// must never have stopped being Primary.
func ValidateVRGState(
	ctx types.TestContext, cluster *types.Cluster,
	desired ramen.ReplicationState, actual ramen.State,
) error {
	return validateVRG(ctx, cluster, desired, actual)
}

// WaitForTestNamespace waits until a test namespace exists on the cluster the
// copy was built on.
func WaitForTestNamespace(ctx types.TestContext, cluster *types.Cluster, name string) error {
	for {
		err := cluster.Client.Get(ctx.Context(), k8stypes.NamespacedName{Name: name}, &corev1.Namespace{})
		if err == nil {
			return nil
		}

		if !k8serrors.IsNotFound(err) {
			return err
		}

		if err := util.Sleep(ctx.Context(), util.RetryInterval); err != nil {
			return fmt.Errorf("test namespace %q not found on cluster %q: %w", name, cluster.Name, err)
		}
	}
}

// DryRunRefusedWhileNotCleanedUp checks that a second test failover cannot be
// started over the first one's leftovers.
//
// Ramen never clears the terminal annotation: leaving it is a deliberate user
// action, and it is the only gate on a repeat run. That matters because every
// run on a DRPC targets the same namespace names, so a second run over
// namespaces nobody deleted would adopt the first copy and report success
// with stale data.
func DryRunRefusedWhileNotCleanedUp(ctx types.TestContext) error {
	managementNamespace := ctx.ManagementNamespace()
	name := ctx.Name()

	drpc, err := getDRPC(ctx, managementNamespace, name)
	if err != nil {
		return err
	}

	phase := ramen.DryRunPhase(drpc.GetAnnotations()[ramen.DryRunAnnotation])
	if phase != ramen.DryRunReady {
		return fmt.Errorf("expected drpc %q to be left at %q, found %q",
			name, ramen.DryRunReady, phase)
	}

	if err := setDryRun(ctx, managementNamespace, name, true); err != nil {
		return err
	}

	defer func() {
		if err := setDryRun(ctx, managementNamespace, name, false); err != nil {
			ctx.Logger().Warnf("Failed to clear dryRun after the refusal check: %s", err)
		}
	}()

	// Nothing should move. The annotation stays terminal and no new run
	// starts, because the previous one was never cleaned up.
	if err := util.Sleep(ctx.Context(), dryRunRefusalWindow); err != nil {
		return err
	}

	drpc, err = getDRPC(ctx, managementNamespace, name)
	if err != nil {
		return err
	}

	if phase := ramen.DryRunPhase(drpc.GetAnnotations()[ramen.DryRunAnnotation]); phase != ramen.DryRunReady {
		return fmt.Errorf("a second test failover started over the first one's leftovers: phase is %q", phase)
	}

	return nil
}

// dryRunRefusalWindow is how long to watch for a second run wrongly starting.
// Long enough for a reconcile or two, short enough not to pad the suite.
const dryRunRefusalWindow = 30 * time.Second
