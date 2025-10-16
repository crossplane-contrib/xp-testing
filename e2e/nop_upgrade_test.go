//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/upgrade"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestUpgradeProvider(t *testing.T) {
	upgradeTest := upgrade.UpgradeTest{
		ClusterName:         kindClusterName,
		ProviderName:        "provider-nop",
		FromProviderPackage: "xpkg.upbound.io/crossplane-contrib/provider-nop:v0.2.0",
		ToProviderPackage:   "xpkg.crossplane.io/crossplane-contrib/provider-nop:v0.4.0",
		ResourceDirectories: []string{
			"crs/Nop",
		},
	}
	mustPullPackage(upgradeTest.FromProviderPackage)
	mustPullPackage(upgradeTest.ToProviderPackage)

	// use the upgrade test feature
	upgradeFeature := upgradeTest.UpgradeFeatureBuilder("upgrade provider-nop from 0.2.0 to 0.4.0", time.Minute*2)

	// build the same upgrade scenario with custom builder and e.g. different timeouts per assessment step
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
		Teardown(upgrade.DeleteResources(upgradeTest.ResourceDirectories, time.Minute*2))

	testenv.Test(t, upgradeFeature.Feature(), customFeatureUpgrade.Feature())
}
