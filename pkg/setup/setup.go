package setup

import (
	"os"
	"strings"

	"github.com/crossplane/crossplane/apis/pkg/v1alpha1"
	"github.com/vladimirvivien/gexe"
	"k8s.io/apimachinery/pkg/runtime"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"

	"github.com/maximilianbraun/xp-testing/pkg/envvar"
	"github.com/maximilianbraun/xp-testing/pkg/images"
	"github.com/maximilianbraun/xp-testing/pkg/xenvfuncs"
)

const (
	reuseClusterEnv = "E2E_REUSE_CLUSTER"
	clusterNameEnv  = "E2E_CLUSTER_NAME"
	defaultPrefix   = "e2e"
)

// ClusterSetup help with a default kind setup for crossplane, with crossplane and a provider
type ClusterSetup struct {
	Name             string
	Images           images.ProviderImages
	ControllerConfig *v1alpha1.ControllerConfig
	SecretData       map[string]string
	AddToSchemaFuncs []func(s *runtime.Scheme) error
}

// Configure optionally creates the kind cluster and takes care about the rest of the setup,
// There are two relevant Environment Variables that influence its behavior
// * E2E_REUSE_CLUSTER: if set, the cluster, crossplane and provider will be reused and not deleted after test.
// If set, CLUSTER_NAME will be ignored
// * TESTCLUSTER_NAME: overwrites the cluster name
func (s *ClusterSetup) Configure(testEnv env.Environment) {

	reuseCluster := CheckEnvVarExists(reuseClusterEnv)
	log.V(4).Info("Reusing cluster: ", reuseCluster)
	kindClusterName := clusterName(reuseCluster)
	log.V(4).Info("Cluster name: ", kindClusterName)
	firstSetup := true
	if reuseCluster && clusterExists(kindClusterName) {
		firstSetup = false
	}

	log.V(4).Info("Is first setup: ", firstSetup)

	// Setup uses pre-defined funcs to create kind cluster
	// and create a namespace for the environment
	testEnv.Setup(
		envfuncs.CreateKindCluster(kindClusterName),
		xenvfuncs.Conditional(xenvfuncs.InstallCrossplane(kindClusterName), firstSetup),
		xenvfuncs.Conditional(
			xenvfuncs.InstallCrossplaneProvider(
				kindClusterName, xenvfuncs.InstallCrossplaneProviderOptions{
					Name:             s.Name,
					Package:          s.Images.Package,
					ControllerImage:  s.Images.ControllerImage,
					ControllerConfig: s.ControllerConfig,
				},
			), firstSetup,
		),
		xenvfuncs.ApplySecretInCrossplaneNamespace("secret", s.SecretData),
		xenvfuncs.ApplyProviderConfig,
		xenvfuncs.LoadSchemas(s.AddToSchemaFuncs...),
		xenvfuncs.AwaitCRDsEstablished,
	)

	// Finish uses pre-defined funcs to
	// remove namespace, then delete cluster
	testEnv.Finish(
		xenvfuncs.DumpKindLogs(kindClusterName),
		xenvfuncs.DeleteTestNamespace,
		xenvfuncs.Conditional(envfuncs.DestroyKindCluster(kindClusterName), !reuseCluster),
	)
}

func clusterName(reuseCluster bool) string {
	var kindClusterName string
	if CheckEnvVarExists(clusterNameEnv) {
		kindClusterName = envvar.GetOrPanic(clusterNameEnv)
	} else if reuseCluster {
		kindClusterName = defaultPrefix
	} else {
		kindClusterName = envvar.GetOrDefault(clusterNameEnv, envconf.RandomName(defaultPrefix, 10))
	}
	return kindClusterName
}

// TODO: Maybe part of the k8s-e2e framework?
func clusterExists(name string) bool {
	e := gexe.New()
	clusters := e.Run("kind get clusters")
	for _, c := range strings.Split(clusters, "\n") {
		if c == name {
			return true
		}
	}
	return false
}

// CheckEnvVarExists returns if a environment variable exists
func CheckEnvVarExists(existsKey string) bool {
	_, found := os.LookupEnv(existsKey)
	return found
}
