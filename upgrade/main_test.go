//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/vladimirvivien/gexe"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kind"

	"sigs.k8s.io/e2e-framework/pkg/env"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
)

var testenv env.Environment
var kindClusterName string

const fromPackage = "xpkg.upbound.io/crossplane-contrib/provider-nop:v0.2.0"
const toPackage = "xpkg.crossplane.io/crossplane-contrib/provider-nop:v0.4.0"

// The environment setup for upgrade tests uses existing e2e setup functionality to create a kind cluster, install crossplane, etc.
// The main differences compared to regular e2e provider tests are the following:
// 1. upgrade tests must be able to install multiple provider versions
// 2. upgrade tests must not run in parallel on the same cluster
func TestMain(m *testing.M) {
	var verbosity = 4
	logging.EnableVerboseLogging(&verbosity)
	testenv = env.New()

	mustPullPackage(fromPackage)
	mustPullPackage(toPackage)

	imgs := images.ProviderImages{
		Package: fromPackage,
	}
	imgs.ControllerImage = &imgs.Package
	clusterSetup := setup.ClusterSetup{
		ProviderName: "provider-nop",
		Images:       imgs,
	}
	clusterSetup.PostCreate(func(clusterName string) env.Func {
		kindClusterName = clusterName
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			klog.V(4).Infof("upgrade cluster %s has been created", clusterName)
			return ctx, nil
		}
	})
	_ = clusterSetup.Configure(testenv, &kind.Cluster{})
	os.Exit(testenv.Run(m))
}

func mustPullPackage(image string) {
	klog.Info("Pulling ", image)
	runner := gexe.New()
	p := runner.RunProc(fmt.Sprintf("docker pull %s", image))
	klog.V(4).Info(p.Out())
	if p.Err() != nil {
		panic(fmt.Errorf("docker pull %v failed: %w: %s", image, p.Err(), p.Result()))
	}
}
