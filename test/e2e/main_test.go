//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/vladimirvivien/gexe"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kind"

	"sigs.k8s.io/e2e-framework/pkg/env"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
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
	mustPullPackage(imgs.Package)

	// Enhance interface for one- based providers
	clusterSetup := setup.ClusterSetup{
		ProviderName:    "provider-nop",
		Images:          imgs,
		CrossplaneSetup: setup.CrossplaneSetup{},
		DeploymentRuntimeConfig: &vendored.DeploymentRuntimeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "provider-nop",
			},
			Spec: vendored.DeploymentRuntimeConfigSpec{
				DeploymentTemplate: &vendored.DeploymentTemplate{
					Spec: &v1.DeploymentSpec{
						Selector: &metav1.LabelSelector{},
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: imgs.Package,
										Name:  "package-runtime",
									},
								},
							},
						},
					},
				},
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

func mustPullPackage(image string) {
	klog.Info("Pulling ", image)
	runner := gexe.New()
	p := runner.RunProc(fmt.Sprintf("docker pull %s", image))
	klog.V(4).Info(p.Out())
	if p.Err() != nil {
		panic(fmt.Errorf("docker pull %v failed: %w: %s", image, p.Err(), p.Result()))
	}
}
