package xpenvfuncs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
	"github.com/crossplane/crossplane/apis/pkg/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1extensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/third_party/helm"
	"sigs.k8s.io/kind/pkg/cluster"

	resHelper "github.com/crossplane-contrib/xp-testing/pkg/resources"
	xconditions "github.com/crossplane-contrib/xp-testing/pkg/xpconditions"

	"github.com/crossplane-contrib/xp-testing/internal/docker"
	"github.com/crossplane-contrib/xp-testing/internal/xpkg"
	"github.com/crossplane-contrib/xp-testing/pkg/xpconditions"
)

const crsCrossplaneCacheVolumeTemplate = `apiVersion: v1
kind: PersistentVolume
metadata:
  name: {{.CacheVolume}}
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 5Mi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: "{{.CacheMount}}"
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{.CacheVolume}}
  namespace: crossplane-system
spec:
  accessModes:
    - ReadWriteOnce
  volumeName: {{.CacheVolume}}
  storageClassName: manual
  resources:
    requests:
      storage: 1Mi`

const crsCrossplaneProviderTemplate = `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {{.Name}}
spec:
  package: {{.Name}}
  packagePullPolicy: Never
  {{- if .ControllerConfig }}
  controllerConfigRef:
    name: {{.ControllerConfig}}
{{end}}`

const (
	helmRepoName = "e2e_crossplane-stable"
	// CrossplaneNamespace the namespace crossplane will be installed to
	CrossplaneNamespace = "crossplane-system"
	clusterNameKey      = "clusterNameType"
)

var (
	// The default key to put the cluster name into context
	defaultClusterKey = clusterNameType(clusterNameKey)
)

type clusterNameType string

// InstallCrossplane returns an env.Func that is used to install crossplane into the given cluster
func InstallCrossplane(clusterName string, crossplaneVersion string) env.Func {
	cacheName := "package-cache"

	return Compose(
		envfuncs.CreateNamespace(CrossplaneNamespace),
		setupCrossplanePackageCache(clusterName, cacheName),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			kindCluster, ok := envfuncs.GetClusterFromContext(ctx, clusterName)
			if !ok {
				return ctx, fmt.Errorf("install crossplane func: cluster '%s' doesn't exist", clusterName)
			}

			manager := helm.New(kindCluster.GetKubeconfig())
			if err := manager.RunRepo(
				helm.WithArgs(
					"add",
					helmRepoName,
					"https://charts.crossplane.io/stable",
					"--force-update",
				),
			); err != nil {
				return ctx, errors.Wrap(err, "install crossplane func: failed to add crossplane helm chart repo")
			}
			if err := manager.RunRepo(helm.WithArgs("update")); err != nil {
				return ctx, errors.Wrap(err, "install crossplane func: failed to upgrade helm repo")
			}

			helmInstallOpts := []helm.Option{
				helm.WithName("crossplane"),
				helm.WithNamespace("crossplane-system"),
				helm.WithVersion(crossplaneVersion),
				helm.WithReleaseName(helmRepoName + "/crossplane"),
				helm.WithArgs("--set", fmt.Sprintf("packageCache.pvc=%s", cacheName)),
				helm.WithTimeout("10m"),
				helm.WithWait(),
			}

			if err := manager.RunInstall(helmInstallOpts...); err != nil {
				return ctx, errors.Wrap(err, "install crossplane func: failed to install crossplane Helm chart")
			}

			return ctx, nil
		},
	)
}

// ApplySecretInCrossplaneNamespace creates secret that is used by providers in the crossplane namespace
func ApplySecretInCrossplaneNamespace(name string, data map[string]string) env.Func {
	return Compose(
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			r, err := resources.New(cfg.Client().RESTConfig())

			if err != nil {
				klog.Error(err)
				return ctx, err
			}

			secret := newSecret(name, data)

			if err := r.Create(ctx, secret); err != nil {
				klog.Error(err)
				return ctx, err
			}

			return ctx, nil
		},
	)
}

// InstallCrossplaneProviderOptions hols information on the tested provider
type InstallCrossplaneProviderOptions struct {
	Name             string
	Package          string
	ControllerImage  *string // TODO read from package
	ControllerConfig *v1alpha1.ControllerConfig
}

// InstallCrossplaneProvider returns an env.Func that is used to
// install a crossplane provider into the active cluster
func InstallCrossplaneProvider(clusterName string, opts InstallCrossplaneProviderOptions) env.Func {
	return Compose(
		loadCrossplanePackageToCluster(clusterName, opts),
		loadCrossplaneControllerImageToCluster(clusterName, opts),
		installCrossplaneProviderEnvFunc(clusterName, opts),
		awaitProviderHealthy(opts),
	)
}

// ApplyProviderConfig applys the files from `./data/provider` and mutates their namespace
func ApplyProviderConfig(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	r, _ := resources.New(cfg.Client().RESTConfig())

	klog.Info("Apply ProviderConfig")
	errDecode := decoder.DecodeEachFile(
		ctx, os.DirFS("./provider"), "*",
		decoder.CreateHandler(r),
		decoder.MutateNamespace(cfg.Namespace()),
	)

	if errDecode != nil {
		klog.Error("Error Details ", "errDecode ", errDecode)
	}

	return ctx, nil
}

// LoadSchemas prepares the kubernetes client with additional schemas
func LoadSchemas(addToSchemaFuncs ...func(s *runtime.Scheme) error) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		r, err := resources.New(cfg.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}
		for _, addToSchemaFunc := range addToSchemaFuncs {
			if err = addToSchemaFunc(r.GetScheme()); err != nil {
				return ctx, err
			}
		}

		if err = v1extensions.AddToScheme(r.GetScheme()); err != nil {
			return ctx, err
		}

		if err = corev1.AddToScheme(r.GetScheme()); err != nil {
			return ctx, err
		}

		return ctx, nil
	}
}

// setupCrossplanePackageCache prepares the crossplane package-cache in the given clusters control plane
func setupCrossplanePackageCache(clusterName string, cacheName string) env.Func {
	cacheMount := "/cache"
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		controlPlaneName := getClusterControlPlaneName(clusterName)

		if err := docker.Exec(controlPlaneName, "mkdir", "-m", "777", "-p", cacheMount); err != nil {
			return ctx, err
		}

		rendered, err := renderTemplate(
			crsCrossplaneCacheVolumeTemplate, struct {
				CacheVolume string
				CacheMount  string
			}{
				CacheVolume: cacheName,
				CacheMount:  cacheMount,
			},
		)

		if err != nil {
			return ctx, err
		}

		return applyResources(ctx, cfg, rendered)
	}
}

// loadCrossplanePackageToCluster loads the crossplane config package into the given clusters package cache folder (/cache)
func loadCrossplanePackageToCluster(clusterName string, opts InstallCrossplaneProviderOptions) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		f, err := os.CreateTemp("", "xpkg")
		if err != nil {
			return ctx, err
		}
		defer func(name string) {
			_ = os.Remove(name)
		}(f.Name())

		clusterControlPlaneName := getClusterControlPlaneName(clusterName)

		if err = xpkg.SavePackage(opts.Package, f.Name()); err != nil {
			return ctx, err
		}

		cachePackagePath := fmt.Sprintf("/cache/%s.gz", opts.Name)

		if err = docker.Cp(f.Name(), fmt.Sprintf("%s:%s", clusterControlPlaneName, cachePackagePath)); err != nil {
			return ctx, err
		}

		return ctx, docker.Exec(clusterControlPlaneName, "chmod", "644", cachePackagePath)
	}
}

// loadCrossplaneControllerImageToCluster loads the controller image into the oci cache of the given cluster
func loadCrossplaneControllerImageToCluster(clusterName string, opts InstallCrossplaneProviderOptions) env.Func {
	if opts.ControllerImage == nil {
		// no-op
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			return ctx, nil
		}
	}
	return envfuncs.LoadDockerImageToCluster(clusterName, *opts.ControllerImage)
}

// installCrossplaneProviderEnvFunc is an env.Func to install a crossplane provider into the given cluster
func installCrossplaneProviderEnvFunc(_ string, opts InstallCrossplaneProviderOptions) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {

		data := struct {
			Name             string
			Package          string
			ControllerConfig string
		}{
			Name:    opts.Name,
			Package: opts.Package,
		}

		if opts.ControllerConfig != nil {
			config := opts.ControllerConfig.DeepCopy()
			config.ObjectMeta = metav1.ObjectMeta{
				Name: opts.Name,
			}
			data.ControllerConfig = opts.Name
			res := cfg.Client().Resources()
			if err := v1alpha1.AddToScheme(res.GetScheme()); err != nil {
				return ctx, err
			}
			err := res.Create(ctx, config)
			if err != nil {
				return nil, err
			}
		}

		crs, err := renderTemplate(
			crsCrossplaneProviderTemplate, data,
		)

		if err != nil {
			return ctx, err
		}
		return applyResources(ctx, cfg, crs)
	}
}

func awaitProviderHealthy(opts InstallCrossplaneProviderOptions) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		provider := pkgv1.Provider{
			ObjectMeta: metav1.ObjectMeta{Name: opts.Name},
		}
		r, err := resources.New(cfg.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}
		if err := pkgv1.AddToScheme(r.GetScheme()); err != nil {
			return ctx, err
		}
		err = wait.For(
			xconditions.New(r).ProviderConditionMatch(
				&provider,
				pkgv1.TypeHealthy,
				corev1.ConditionTrue,
			), wait.WithTimeout(time.Minute*5),
		)
		return ctx, err
	}
}

// applyResources is an equivalent for kubectl apply
func applyResources(ctx context.Context, cfg *envconf.Config, crs string) (context.Context, error) {
	r, err := resources.New(cfg.Client().RESTConfig())
	if err != nil {
		return ctx, err
	}

	return ctx, decoder.DecodeEach(ctx, strings.NewReader(crs), decoder.CreateHandler(r))
}

// getClusterControlPlaneName returns the supposed name of the given clusters control plane
func getClusterControlPlaneName(clusterName string) string {
	return fmt.Sprintf("%s-control-plane", clusterName)
}

// Compose executes multiple env.Funcs in a row
func Compose(envfuncs ...env.Func) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		for _, envfunc := range envfuncs {
			if ctx, err := envfunc(ctx, cfg); err != nil {
				return ctx, err
			}
		}
		return ctx, nil
	}
}

func renderTemplate(tmpl string, data interface{}) (string, error) {
	h := sha256.New()
	_, _ = io.WriteString(h, tmpl)

	hash := string(h.Sum(nil))

	parsedTmpl, err := template.New(hash).Parse(tmpl)
	if err != nil {
		return "", err
	}

	buf := bytes.Buffer{}
	if err := parsedTmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// IgnoreErr exec's fn, logs possible error away continues w/o error
func IgnoreErr(fn env.Func) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		if _, err := fn(ctx, cfg); err != nil {
			klog.V(4).Info("Ignored Err:", err)
		}
		return ctx, nil
	}

}

// IgnoreMatchedErr checks if a result of fn() returns an error and if the error matches result of errorMatcher() ignores the error to continue with execution
func IgnoreMatchedErr(fn env.Func, errorMatcher func(err error) bool) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		if _, err := fn(ctx, cfg); err != nil && errorMatcher(err) {
			klog.V(4).Info("Ignored Err:", err)
		}
		return ctx, nil
	}

}

// Conditional executes a fn based on conditional
func Conditional(fn env.Func, condition bool) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		if condition {
			return fn(ctx, cfg)
		}
		return ctx, nil
	}
}

// Create Opaque secret from non-binary data in crossplane namespace
func newSecret(name string, stringData map[string]string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: CrossplaneNamespace,
		},
		StringData: stringData,
		Type:       corev1.SecretTypeOpaque,
	}

	return secret
}

// AwaitCRDsEstablished waits until all CRDs do have a condition `Established` == true
func AwaitCRDsEstablished(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	client, err := resources.New(cfg.Client().RESTConfig())
	if err != nil {
		return ctx, err
	}
	var crds v1extensions.CustomResourceDefinitionList

	if err := client.List(ctx, &crds); err != nil {
		return ctx, err
	}

	c := xpconditions.New(client)
	err = wait.For(
		c.ResourcesMatch(&crds, crdIsEstablished), wait.WithTimeout(time.Minute),
	)
	return ctx, err
}

func crdIsEstablished(object k8s.Object) bool {
	crd, ok := object.(*v1extensions.CustomResourceDefinition)
	if !ok {
		panic("No CRD with this object")
	}

	for _, condition := range crd.Status.Conditions {
		if condition.Type != v1extensions.Established {
			continue
		}
		want := v1extensions.ConditionTrue
		got := condition.Status

		klog.V(4).Infof(
			"Checking resource (%s) condition %s, got %s, want %s",
			resHelper.Identifier(crd),
			v1extensions.Established,
			got,
			want,
		)
		return got == want

	}
	return false
}

// DumpKindLogs Dumps the logs of the cluster to `$PWD/logs` using kind export func
func DumpKindLogs(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		provider := cluster.NewProvider()
		dir, err := os.Getwd()
		if err != nil {
			return ctx, err
		}
		effectivePath := path.Join(dir, "logs")
		klog.Infof("Writing kind logs to %s", effectivePath)
		err = provider.CollectLogs(clusterName, effectivePath)

		return ctx, err
	}
}

// DumpKubernetesLogs Dumps the logs of the cluster to `$PWD/logs-$identifier` using kind export func
func DumpKubernetesLogs(ctx context.Context, identifier string) (context.Context, error) {
	provider := cluster.NewProvider()
	dir, err := os.Getwd()
	if err != nil {
		return ctx, err
	}
	clusterName, ok := ctx.Value(defaultClusterKey).(string)
	if !ok {
		return ctx, errors.New("cluster not found in test context")
	}
	effectivePath := path.Join(dir, fmt.Sprintf("logs-%s", identifier))
	klog.Infof("Writing kind logs to %s", effectivePath)
	err = provider.CollectLogs(clusterName, effectivePath)

	return ctx, err
}

// SetClusterName Sets the name of the cluster into context to retrieve it by other functions
func SetClusterName(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		ctx = context.WithValue(ctx, defaultClusterKey, clusterName)

		return ctx, nil
	}
}

// CreateTestNamespace Creates the test namespace, name comes from kubernetes-e2e
func CreateTestNamespace(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	return envfuncs.CreateNamespace(cfg.Namespace())(ctx, cfg)
}

// DeleteTestNamespace Deletes the test namespace, name comes from kubernetes-e2e
func DeleteTestNamespace(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	return envfuncs.DeleteNamespace(cfg.Namespace())(ctx, cfg)
}
