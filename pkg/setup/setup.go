package setup

import (
	"context"
	"os"
	"strings"

	"github.com/vladimirvivien/gexe"
	"k8s.io/apimachinery/pkg/runtime"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
)

const (
	reuseClusterEnv = "E2E_REUSE_CLUSTER"
	clusterNameEnv  = "E2E_CLUSTER_NAME"
	defaultPrefix   = "e2e"

	// DockerRegistry is the default docker registry, which can be passed to the crossplane setup (prior to v2)
	DockerRegistry = "index.docker.io"
)

// ProviderCredentials holds the data for a secret to be created in the crossplane namespace
type ProviderCredentials struct {
	SecretData map[string]string
	SecretName *string
}

// CrossplaneSetup holds configuration specific to the crossplane installation
type CrossplaneSetup struct {
	Version  string
	Registry string
	// ChartRef is an optional chart reference passed straight to `helm install`.
	// When non-empty the `helm repo add` / `helm repo update` flow is skipped
	// entirely. Accepts:
	//   - file path: /path/to/crossplane-1.20.1.tgz
	//   - OCI URL:   oci://xpkg.crossplane.io/crossplane/crossplane
	//   - repo/name: e2e_crossplane-stable/crossplane (the legacy default)
	//
	// Useful when the upstream Crossplane chart CDN is unreliable in CI
	// (intermittent 403 Forbidden), in air-gapped environments, or when
	// pulling the chart from an OCI registry instead of a classic helm repo.
	// The caller is responsible for ensuring the ref is reachable by helm.
	// Mutually exclusive with ChartRepoURL — if both are set ChartRef takes
	// precedence and ChartRepoURL is ignored.
	ChartRef string
	// ChartRepoURL optionally overrides the default
	// https://charts.crossplane.io/stable helm repository URL. Ignored when
	// ChartRef is set.
	ChartRepoURL string
}

// Options returns configurtion as options pattern to be passed on to installation process step
func (c CrossplaneSetup) Options() []xpenvfuncs.CrossplaneOpt {
	var opts []xpenvfuncs.CrossplaneOpt
	if c.Version != "" {
		opts = append(opts, xpenvfuncs.Version(c.Version))
	}
	if c.Registry != "" {
		opts = append(opts, xpenvfuncs.Registry(c.Registry))
	}
	return opts
}

// installCrossplaneFunc returns the env.Func that performs the crossplane
// installation, branching on ChartRef / ChartRepoURL to pick the right entry
// point. ChartRef takes precedence over ChartRepoURL when both are set.
func (c CrossplaneSetup) installCrossplaneFunc(clusterName string) env.Func {
	switch {
	case c.ChartRef != "":
		return xpenvfuncs.InstallCrossplaneFromChart(clusterName, c.ChartRef, c.Options()...)
	case c.ChartRepoURL != "":
		return xpenvfuncs.InstallCrossplaneFromRepo(clusterName, c.ChartRepoURL, c.Options()...)
	default:
		return xpenvfuncs.InstallCrossplane(clusterName, c.Options()...)
	}
}

// ClusterSetup help with a default kind setup for crossplane, with crossplane and a provider
type ClusterSetup struct {
	ProviderName    string
	Images          images.ProviderImages
	CrossplaneSetup CrossplaneSetup
	// CrossplaneInstallFunc, if non-nil, replaces the bundled InstallCrossplane
	// step in Configure. The function is invoked once per Configure call (when
	// firstSetup is true, i.e., not reusing an existing cluster) before
	// InstallCrossplaneProvider runs.
	//
	// The replacement is responsible for satisfying the package-cache contract:
	//
	//   1. Create namespace "crossplane-system".
	//   2. Set up a PV+PVC backed by /cache/xpkg on the kind control-plane node
	//      (see SetupCrossplanePackageCache when that helper is exported, or
	//      replicate inline using xpenvfuncs.setupCrossplanePackageCache's
	//      logic).
	//   3. `helm install crossplane <chartRef> --set packageCache.pvc=<name>`
	//      where <name> matches the PVC created in step 2.
	//
	// Without these, InstallCrossplaneProvider's loadCrossplanePackageToCluster
	// deposits the xpkg into a host directory the Crossplane pod can't read,
	// and providers fail Healthy with:
	//
	//     failed to get pre-cached package with pull policy Never
	//
	// Use this hook for:
	//   - Installing Crossplane from a pre-pulled local chart tarball
	//     (avoiding `helm repo add charts.crossplane.io/stable` flake)
	//   - Pulling from an OCI registry (oci://...)
	//   - Pulling from a custom helm repo URL
	//   - Air-gapped environments
	//
	// The CrossplaneSetup.Version / Registry / ChartRef / ChartRepoURL fields
	// are ignored when CrossplaneInstallFunc is set — the caller has full
	// control.
	CrossplaneInstallFunc   env.Func
	ControllerConfig        *vendored.ControllerConfig
	DeploymentRuntimeConfig *vendored.DeploymentRuntimeConfig
	ProviderCredential      *ProviderCredentials
	AddToSchemaFuncs        []func(s *runtime.Scheme) error
	postSetupFuncs          []ClusterAwareFunc
	ProviderConfigDir       *string
}

// Configure optionally creates the kind cluster and takes care about the rest of the setup,
// There are two relevant Environment Variables that influence its behavior
// * E2E_REUSE_CLUSTER: if set, the cluster, crossplane and provider will be reused and not deleted after test.
// If set, CLUSTER_NAME will be ignored
// * E2E_CLUSTER_NAME: overwrites the cluster name
// Currently requires a kind.Cluster, only for kind we can detect if a cluster is reusable
// nolint:interfacer
func (s *ClusterSetup) Configure(testEnv env.Environment, cluster *kind.Cluster) string {
	reuseCluster := envvar.CheckEnvVarExists(reuseClusterEnv)
	log.V(4).Info("Reusing cluster: ", reuseCluster)
	name := clusterName(reuseCluster)
	log.V(4).Info("Cluster name: ", name)
	firstSetup := true
	if reuseCluster && kindClusterExists(name) {
		firstSetup = false
	}

	log.V(4).Info("Is first setup: ", firstSetup)
	// Setup uses pre-defined funcs to create kind cluster
	// and create a namespace for the environment

	testEnv.Setup(
		xpenvfuncs.ValidateTestSetup(xpenvfuncs.ValidateTestSetupOptions{
			CrossplaneVersion: s.CrossplaneSetup.Version,
			PackageRegistry:   s.CrossplaneSetup.Registry,
			ControllerConfig:  s.ControllerConfig,
		}),
		envfuncs.CreateCluster(cluster, name),
	)
	for _, claFunc := range s.postSetupFuncs {
		testEnv.Setup(claFunc(name))
	}
	testEnv.Setup(
		xpenvfuncs.Conditional(
			xpenvfuncs.Compose(
				s.CrossplaneSetup.installCrossplaneFunc(name),
				xpenvfuncs.InstallCrossplaneProvider(
					name, xpenvfuncs.InstallCrossplaneProviderOptions{
						Name:                    s.ProviderName,
						Package:                 s.Images.Package,
						ControllerImage:         s.Images.ControllerImage,
						ControllerConfig:        s.ControllerConfig,
						DeploymentRuntimeConfig: s.DeploymentRuntimeConfig,
					}),
			), firstSetup),
		setupProviderCredentials(s),
		xpenvfuncs.ApplyProviderConfigFromDir(orDefault(s.ProviderConfigDir, "./provider")),
		xpenvfuncs.LoadSchemas(s.AddToSchemaFuncs...),
		xpenvfuncs.AwaitCRDsEstablished)

	// Finish uses pre-defined funcs to
	// remove namespace, then delete cluster
	testEnv.Finish(
		xpenvfuncs.DumpLogs(name, "post-tests"),
		xpenvfuncs.Conditional(envfuncs.DestroyCluster(name), !reuseCluster),
	)
	return name
}

// installCrossplaneFunc returns the env.Func that installs the Crossplane
// control plane. When CrossplaneInstallFunc is non-nil, it takes precedence
// over the bundled InstallCrossplane.
func (s *ClusterSetup) installCrossplaneFunc(clusterName string) env.Func {
	if s.CrossplaneInstallFunc != nil {
		return s.CrossplaneInstallFunc
	}
	return xpenvfuncs.InstallCrossplane(clusterName, s.CrossplaneSetup.Options()...)
}

func setupProviderCredentials(s *ClusterSetup) env.Func {
	if s.ProviderCredential == nil {
		return nil
	}
	return xpenvfuncs.ApplySecretInCrossplaneNamespace(
		orDefault(s.ProviderCredential.SecretName, "secret"),
		s.ProviderCredential.SecretData)
}

func orDefault(overwriteValue *string, defaultValue string) string {
	if overwriteValue == nil {
		return defaultValue
	}
	return *overwriteValue
}

// ClusterAwareFunc are functions which create env.Func and have the clusters name as context
type ClusterAwareFunc = func(clusterName string) env.Func

// PostCreate registers ClusterAwareFunc to run after Cluster creation
func (s *ClusterSetup) PostCreate(funcs ...ClusterAwareFunc) {
	s.postSetupFuncs = funcs
}

func clusterName(reuseCluster bool) string {

	if envvar.CheckEnvVarExists(clusterNameEnv) {
		return os.Getenv(clusterNameEnv)
	}

	if reuseCluster {
		return defaultPrefix
	}

	return envconf.RandomName(defaultPrefix, 10)
}

// TODO: Maybe part of the k8s-e2e framework?
func kindClusterExists(name string) bool {
	e := gexe.New()
	envfuncs.GetClusterFromContext(context.TODO(), name)
	clusters := e.Run("kind get clusters")
	for _, c := range strings.Split(clusters, "\n") {
		if c == name {
			return true
		}
	}
	return false
}
