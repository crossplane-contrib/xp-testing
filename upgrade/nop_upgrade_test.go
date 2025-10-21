//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/upgrade"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// TestUpgradProviderFeature demonstrates usage of the UpgradeFeatureBuilder
func TestUpgradeProviderFeature(t *testing.T) {
	upgradeTest := upgrade.UpgradeTest{
		ClusterName:         kindClusterName,
		ProviderName:        "provider-nop",
		FromProviderPackage: fromPackage,
		ToProviderPackage:   toPackage,
		ResourceDirectories: []string{
			"../e2e/crs/Nop",
		},
	}

	// use the upgrade test feature
	upgradeFeature := upgradeTest.UpgradeFeatureBuilder("upgrade provider-nop from 0.2.0 to 0.4.0", time.Minute*2)
	testenv.Test(t, upgradeFeature.Feature())
}

// TestUpgradProviderCustom demonstrates how to build a custom upgrade feature
func TestUpgradeProviderCustom(t *testing.T) {
	upgradeTest := upgrade.UpgradeTest{
		ClusterName:         kindClusterName,
		ProviderName:        "provider-nop",
		FromProviderPackage: fromPackage,
		ToProviderPackage:   toPackage,
		ResourceDirectories: []string{
			"../e2e/crs/Nop",
		},
	}

	// build the UpgradeFeatureBuilder scenario with a custom builder and e.g. different timeouts per assessment step
	// this enables you to implement additional setup, assessment and teardown steps like more sophisticated pre/post upgrade conditions, etc.
	// or to completely re-orchestrate the upgrade steps
	customFeatureUpgrade := features.New("custom build upgrade").
		Setup(upgrade.ApplyProvider(kindClusterName, upgradeTest.FromProviderInstallOptions())).
		Setup(upgrade.ImportResources(upgradeTest.ResourceDirectories)).
		Assess("verify resources before upgrade", upgrade.VerifyResources(upgradeTest.ResourceDirectories, time.Minute*4)).
		Assess("upgrade provider", upgrade.UpgradeProvider(upgrade.UpgradeProviderOptions{
			ClusterName:         kindClusterName,
			ProviderOptions:     upgradeTest.ToProviderInstallOptions(),
			ResourceDirectories: upgradeTest.ResourceDirectories,
			WaitForPause:        time.Minute * 1,
		})).
		Assess("verify resources after upgrade", upgrade.VerifyResources(upgradeTest.ResourceDirectories, time.Minute*3)).
		Teardown(upgrade.DeleteResources(upgradeTest.ResourceDirectories, time.Minute*2)).
		Teardown(upgrade.DeleteProvider(upgradeTest.ProviderName))

	testenv.Test(t, customFeatureUpgrade.Feature())
}
