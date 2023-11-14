package setup

import (
	"context"
	"os"
	"strings"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane/crossplane/apis/pkg/v1alpha1"
	"github.com/vladimirvivien/gexe"
	"k8s.io/apimachinery/pkg/runtime"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
)

const (
	reuseClusterEnv = "E2E_REUSE_CLUSTER"
	clusterNameEnv  = "E2E_CLUSTER_NAME"
	defaultPrefix   = "e2e"
)

// ClusterSetup help with a default kind setup for crossplane, with crossplane and a provider
type ClusterSetup struct {
	Name              string
	Images            images.ProviderImages
	ControllerConfig  *v1alpha1.ControllerConfig
	SecretData        map[string]string
	AddToSchemaFuncs  []func(s *runtime.Scheme) error
	CrossplaneVersion string
}

// Configure optionally creates the kind cluster and takes care about the rest of the setup,
// There are two relevant Environment Variables that influence its behavior
// * E2E_REUSE_CLUSTER: if set, the cluster, crossplane and provider will be reused and not deleted after test.
// If set, CLUSTER_NAME will be ignored
// * TESTCLUSTER_NAME: overwrites the cluster name
// Currently requires a kind.Cluster, only for kind we can detect if a cluster is reusable
// nolint:interfacer
func (s *ClusterSetup) Configure(testEnv env.Environment, cluster *kind.Cluster) {

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
		envfuncs.CreateCluster(cluster, name),
		xpenvfuncs.Conditional(
			xpenvfuncs.Compose(
				xpenvfuncs.InstallCrossplane(name, s.CrossplaneVersion),
				xpenvfuncs.InstallCrossplaneProvider(
					name, xpenvfuncs.InstallCrossplaneProviderOptions{
						Name:             s.Name,
						Package:          s.Images.Package,
						ControllerImage:  s.Images.ControllerImage,
						ControllerConfig: s.ControllerConfig,
					}),
				xpenvfuncs.ApplySecretInCrossplaneNamespace("secret", s.SecretData),
			), firstSetup),
		xpenvfuncs.ApplyProviderConfig,
		xpenvfuncs.LoadSchemas(s.AddToSchemaFuncs...),
		xpenvfuncs.AwaitCRDsEstablished,
	)

	// Finish uses pre-defined funcs to
	// remove namespace, then delete cluster
	testEnv.Finish(
		xpenvfuncs.DumpLogs(name, "post-tests"),
		xpenvfuncs.Conditional(envfuncs.DestroyCluster(name), !reuseCluster),
	)
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
