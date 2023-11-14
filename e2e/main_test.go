//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/vladimirvivien/gexe"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kind"

	xpv1alpha1 "github.com/crossplane/crossplane/apis/pkg/v1alpha1"
	"sigs.k8s.io/e2e-framework/pkg/env"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	var verbosity = 4
	logging.EnableVerboseLogging(&verbosity)
	testenv = env.NewParallel()

	imgs := images.ProviderImages{
		Package: "xpkg.upbound.io/crossplane-contrib/provider-nop:v0.2.0",
	}
	imgs.ControllerImage = &imgs.Package

	// We pull the image here, usually this would have been done by the build tool
	pullPackageOrPanic(imgs.Package)

	// Enhance interface for one- based providers
	clusterSetup := setup.ClusterSetup{
		ProviderName:      "provider-nop",
		Images:            imgs,
		CrossplaneVersion: "1.14.0",
		ControllerConfig: &xpv1alpha1.ControllerConfig{
			Spec: xpv1alpha1.ControllerConfigSpec{
				Image: &imgs.Package,
			},
		},
	}
	clusterSetup.PostCreate(func(clusterName string) env.Func {
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			klog.V(4).Infof("Some function running after the cluster %s has been created", clusterName)
			return ctx, nil
		}
	})
	_ = clusterSetup.Configure(testenv, &kind.Cluster{})
	os.Exit(testenv.Run(m))
}

func pullPackageOrPanic(image string) {
	klog.Info("Pulling %s", image)
	runner := gexe.New()
	p := runner.RunProc(fmt.Sprintf("docker pull %s", image))
	klog.V(4).Info(p.Out())
	if p.Err() != nil {
		panic(fmt.Errorf("docker pull %v failed: %w: %s", image, p.Err(), p.Result()))
	}
}
