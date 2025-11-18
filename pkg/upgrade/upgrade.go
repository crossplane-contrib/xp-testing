package upgrade

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	fwresources "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// UpgradeTest represents the data in a typical provider upgrade scenario where a set of resources
// must remain synced and ready when a provider is upgraded from version x to version y.
type UpgradeTest struct {
	// ClusterName identifies the kind cluster to use
	ClusterName string
	// ProviderName is used as the provider metadata.name
	ProviderName string
	// FromProviderPackage is the package that is being requested for the provider to upgrade from
	FromProviderPackage string
	// FromProviderRuntimeConfig is the optional runtime config for the provider to upgrade from
	FromProviderRuntimeConfig *vendored.DeploymentRuntimeConfig
	// ToProviderPackage is the package that is being requested for the provider to upgrade to
	ToProviderPackage string
	// ToProviderRuntimeConfig is the optional runtime config for the provider to upgrade to
	ToProviderRuntimeConfig *vendored.DeploymentRuntimeConfig
	// ResourceDirectories is the set of directories including manifests to assert the provider upgrade with
	ResourceDirectories []string
}

// UpgradeFeatureBuilder provides a complete upgrade test feature builder that can be extended with additional steps and labels.
// Use this for simple upgrade scenarios or reuse the building blocks to orchestrate a custom upgrade feature.
func (ut *UpgradeTest) UpgradeFeatureBuilder(featureName string, timeout time.Duration, setupfuncs ...features.Func) *features.FeatureBuilder {
	return features.New(featureName).
		WithSetup("install provider", ApplyProvider(ut.ClusterName, ut.FromProviderInstallOptions())).
		WithSetup("import resources", ImportResources(ut.ResourceDirectories)).
		Assess("verify resources before upgrade", VerifyResources(ut.ResourceDirectories, timeout)).
		Assess("upgrade provider", UpgradeProvider(UpgradeProviderOptions{
			ClusterName:         ut.ClusterName,
			ProviderOptions:     ut.ToProviderInstallOptions(),
			ResourceDirectories: ut.ResourceDirectories,
			WaitForPause:        timeout,
		})).
		Assess("verify resources after upgrade", VerifyResources(ut.ResourceDirectories, timeout)).
		WithTeardown("delete resources", DeleteResources(ut.ResourceDirectories, timeout)).
		WithTeardown("delete provider", DeleteProvider(ut.ProviderName))
}

// FromProviderInstallOptions assembles provider install options based on the from provider upgrade specs.
func (ut *UpgradeTest) FromProviderInstallOptions() xpenvfuncs.InstallCrossplaneProviderOptions {
	return xpenvfuncs.InstallCrossplaneProviderOptions{
		Name:                    ut.ProviderName,
		Package:                 ut.FromProviderPackage,
		ControllerImage:         &ut.FromProviderPackage,
		DeploymentRuntimeConfig: ut.FromProviderRuntimeConfig,
	}
}

// ToProviderInstallOptions assembles provider install options based on the to provider upgrade specs.
func (ut *UpgradeTest) ToProviderInstallOptions() xpenvfuncs.InstallCrossplaneProviderOptions {
	return xpenvfuncs.InstallCrossplaneProviderOptions{
		Name:                    ut.ProviderName,
		Package:                 ut.ToProviderPackage,
		ControllerImage:         &ut.ToProviderPackage,
		DeploymentRuntimeConfig: ut.ToProviderRuntimeConfig,
	}
}

// PauseResources iterates over each resource directory and pauses every resource.
func PauseResources(directories []string, timeout time.Duration) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		for _, dir := range directories {
			klog.V(4).Infof("pause resources of directory %s", dir)
			// prepare provider upgrade by pausing reconciliation of MRs
			resources.PauseResources(ctx, t, c, dir)
			resourceCfg := resources.ResourceTestConfig{
				ResourceDirectory: dir,
			}
			// results in conditions update by xp runtime
			if err := resources.WaitForResourcesToBePaused(ctx, c, resourceCfg.ResourceDirectory, resourceCfg.ObjFilterFunc, wait.WithTimeout(timeout)); err != nil {
				t.Errorf("pause resources of directory %s failed: %v", dir, err)
			}
		}
		return ctx
	}
}

// ResumeResources iterates over each resource directory and resumes (crossplane.io/paused: false) every resource.
func ResumeResources(directories []string) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		for _, dir := range directories {
			klog.V(4).Infof("resume resources of directory %s", dir)
			// trigger reconcile with new provider version
			resources.ResumeResources(ctx, t, c, dir)
		}
		return ctx
	}
}

// ImportResources iterates over each resource directory and applies every resource to the cluster.
func ImportResources(directories []string) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		for _, dir := range directories {
			klog.V(4).Infof("import resources of directory %s", dir)
			resources.ImportResources(ctx, t, c, dir)
		}
		return ctx
	}
}

// VerifyResources iterates over each resource directory and waits until each resource is synced and ready.
func VerifyResources(directories []string, timeout time.Duration) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		for _, dir := range directories {
			klog.V(4).Infof("verify resources of directory %s", dir)
			resourceCfg := resources.ResourceTestConfig{
				ResourceDirectory: dir,
			}
			if err := resources.WaitForResourcesToBeSynced(ctx, c, resourceCfg.ResourceDirectory, resourceCfg.ObjFilterFunc, wait.WithTimeout(timeout)); err != nil {
				t.Errorf("verify resources of directory %s failed: %v", dir, err)
			}
		}
		return ctx
	}
}

// DeleteResources iterates over each resource directory, deletes each resource and waits for the deletion to be successful.
func DeleteResources(directories []string, timeout time.Duration) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		klog.V(4).Infof("delete resources of directories %s", strings.Join(directories, ", "))
		return resources.DeleteResourcesFromDirs(ctx, t, c, directories, wait.WithTimeout(timeout))
	}
}

// DeleteProvider deletes a provider identified via metadata.name.
func DeleteProvider(providerName string) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		klog.V(4).Infof("delete provider %s", providerName)
		r, err := fwresources.New(c.Client().RESTConfig())
		if err != nil {
			t.Fatalf("failed to create controller runtime client: %v", err)
		}
		providerObj := unstructured.Unstructured{}
		providerObj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "pkg.crossplane.io",
			Version: "v1",
			Kind:    "Provider",
		})
		if err := r.Get(ctx, providerName, "", &providerObj); err != nil {
			t.Errorf("failed to retrieve provider %s for deletion: %v", providerName, err)
			return ctx
		}
		resources.AwaitResourceDeletionOrFail(ctx, t, c, &providerObj)
		return ctx
	}
}

// ApplyProvider installs a crossplane provider in a cluster. If the crossplane provider already exists, the provider is replaced.
func ApplyProvider(clusterName string, opts xpenvfuncs.InstallCrossplaneProviderOptions) features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		klog.V(4).Infof("apply provider: %s", opts.Package)
		installProvider := xpenvfuncs.InstallCrossplaneProvider(clusterName, opts)
		ctx, err := installProvider(ctx, cfg)
		if err != nil {
			resources.DumpManagedResources(ctx, t, cfg)
			t.Fatalf("apply provider %s failed: %v", opts.Package, err)
		}
		return ctx
	}
}

// UpgradeProviderOptions represents the necessary parameters to orchestrate a provider upgrade
type UpgradeProviderOptions struct {
	ClusterName string
	// ProviderOptions defines the provider spec to upgrade to
	ProviderOptions xpenvfuncs.InstallCrossplaneProviderOptions
	// ResourceDirectories defines the resource directories to iterate over to pause any included object before the upgrade
	// and resume any included object after the provider upgrade
	ResourceDirectories []string
	// WaitForPause defines the timeout to wait for all resources to match condition ReconcilePaused
	WaitForPause time.Duration
}

// UpgradeProvider orchestrates an provider upgrade by first pausing all resources,
// then installing the new provider version and finally resuming all resources
func UpgradeProvider(options UpgradeProviderOptions) features.Func {
	return Compose(
		PauseResources(options.ResourceDirectories, options.WaitForPause),
		ApplyProvider(options.ClusterName, options.ProviderOptions),
		ResumeResources(options.ResourceDirectories),
	)
}

// Compose executes multiple features.Funcs in a row
func Compose(featureFuncs ...features.Func) features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		for _, featureFunc := range featureFuncs {
			ctx = featureFunc(ctx, t, cfg)
		}
		return ctx
	}
}
