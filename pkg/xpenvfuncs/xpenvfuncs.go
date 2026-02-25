package xpenvfuncs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	v1extensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/third_party/helm"

	resHelper "github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"

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
  package: {{.Package}}
  packagePullPolicy: Never
  {{- if .ControllerConfig }}
  controllerConfigRef:
    name: {{.ControllerConfig}}
  {{end}}
  {{- if .RuntimeConfig }}
  runtimeConfigRef:
    name: {{.RuntimeConfig}}
  {{end}}
`

const (
	helmRepoName = "e2e_crossplane-stable"
	// CrossplaneNamespace the namespace crossplane will be installed to
	CrossplaneNamespace   = "crossplane-system"
	errNoClusterInContext = "could get get cluster with this name from context"
	// for images that are not pulled from a registry
	localImageDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

var (
	controllerConfigSchema        = schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1alpha1", Resource: "controllerconfigs"}
	deploymentRuntimeConfigSchema = schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1beta1", Resource: "deploymentruntimeconfigs"}
)

// ValidateTestSetupOptions holds information required to validate the test setup
type ValidateTestSetupOptions struct {
	CrossplaneVersion string
	PackageRegistry   string
	ControllerConfig  *vendored.ControllerConfig
}

// validates the test setup to prevent common misconfigurations with the introduction of crossplane v2
func ValidateTestSetup(opts ValidateTestSetupOptions) env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		if !isV2(opts.CrossplaneVersion) {
			return ctx, nil
		}
		if opts.PackageRegistry != "" {
			return nil, errors.New("the registry flag is no longer available in Crossplane v2")
		}
		if opts.ControllerConfig != nil {
			return nil, errors.New("controller config is no longer available in Crossplane v2")
		}
		return ctx, nil
	}
}

// an empty version results in using the latest stable version which is currently v2
func isV2(version string) bool {
	return version == "" || semver.Major(version) == "v2"
}

// InstallCrossplane returns an env.Func that is used to install crossplane into the given cluster
func InstallCrossplane(clusterName string, opts ...CrossplaneOpt) env.Func {
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

			helmInstallOpts := make([]helm.Option, 0, 6+len(opts))
			helmInstallOpts = append(helmInstallOpts,
				helm.WithName("crossplane"),
				helm.WithNamespace("crossplane-system"),
				helm.WithReleaseName(helmRepoName+"/crossplane"),
				helm.WithArgs("--set", fmt.Sprintf("packageCache.pvc=%s", cacheName)),
				helm.WithTimeout("10m"),
				helm.WithWait(),
			)

			helmInstallOpts = append(helmInstallOpts, opts...)

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

			secret := SimpleSecret(name, CrossplaneNamespace, data)

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
	Name                    string
	Package                 string
	ControllerImage         *string // TODO read from package
	ControllerConfig        *vendored.ControllerConfig
	DeploymentRuntimeConfig *vendored.DeploymentRuntimeConfig
}

// InstallCrossplaneProvider returns an env.Func that is used to
// install a crossplane provider into the active cluster
func InstallCrossplaneProvider(clusterName string, opts InstallCrossplaneProviderOptions) env.Func {
	return Compose(
		loadCrossplanePackageToCluster(clusterName, opts.Package),
		loadCrossplaneControllerImageToCluster(clusterName, opts.ControllerImage),
		installCrossplaneProviderEnvFunc(clusterName, opts),
		awaitProviderHealthy(opts.Name),
	)
}

// ApplyProviderConfigFromDir applies the files from given folder and mutates their namespace
func ApplyProviderConfigFromDir(dir string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		r, _ := resources.New(cfg.Client().RESTConfig())
		klog.Info("Apply ProviderConfig")
		errDecode := decoder.DecodeEachFile(
			ctx, os.DirFS(dir), "*",
			decoder.CreateHandler(r),
			decoder.MutateNamespace(cfg.Namespace()),
		)

		if errDecode != nil {
			klog.Error("Error Details ", "errDecode ", errDecode)
		}

		return ctx, nil
	}
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
	cacheMount := "/cache/xpkg"
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
func loadCrossplanePackageToCluster(clusterName string, pkg string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		f, err := os.CreateTemp("", "xpkg")
		if err != nil {
			return ctx, err
		}
		defer func(name string) {
			_ = os.Remove(name)
		}(f.Name())

		clusterControlPlaneName := getClusterControlPlaneName(clusterName)

		if err = xpkg.SavePackage(pkg, f.Name()); err != nil {
			return ctx, err
		}

		digest, err := retrieveDigest(ctx, pkg)
		if err != nil {
			return ctx, err
		}
		var friendlyIdentifier string
		if digest == localImageDigest {
			friendlyIdentifier = friendlyID(pkg, digest)
		} else {
			ref, err := name.ParseReference(pkg)
			if err != nil {
				return ctx, err
			}
			friendlyIdentifier = friendlyID(parsePackageSourceFromReference(ref), digest)
		}

		cacheKeys := []string{
			fullyQualifiedPathName("/cache/xpkg/", pkg, ".gz"),
			fullyQualifiedPathName("/cache/xpkg/", friendlyIdentifier, ".gz"),
		}

		for _, key := range cacheKeys {
			if err := docker.Exec(clusterControlPlaneName, "mkdir", "-m", "777", "-p", filepath.Dir(key)); err != nil {
				return ctx, err
			}
			if err := docker.Cp(f.Name(), fmt.Sprintf("%s:%s", clusterControlPlaneName, key)); err != nil {
				return ctx, err
			}
			if err := docker.Exec(clusterControlPlaneName, "chmod", "644", key); err != nil {
				return ctx, err
			}
		}

		return ctx, nil
	}
}

// (from crossplane internal/xpkg)
func fullyQualifiedPathName(cacheDir, packageName, ext string) string {
	full := filepath.Join(cacheDir, packageName)
	existExt := filepath.Ext(full)

	return full[0:len(full)-len(existExt)] + ext
}

func retrieveDigest(ctx context.Context, img string) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}
	inspect, err := cli.ImageInspect(ctx, img)
	if err != nil {
		return "", err
	}
	if len(inspect.RepoDigests) > 0 {
		repoDigest := inspect.RepoDigests[0]
		spl := strings.Split(repoDigest, "@")
		return spl[1], nil
	}
	return localImageDigest, nil
}

// source: crossplane/internal/xpkg
func parsePackageSourceFromReference(ref name.Reference) string {
	return strings.TrimRight(strings.TrimSuffix(ref.String(), ref.Identifier()), ":@")
}

// friendlyID builds a valid DNS label string made up of the name of a package
// and its image digest.
// source: crossplane/internal/xpkg/name.go
func friendlyID(name, hash string) string {
	return toDNSLabel(strings.Join([]string{truncate(name, 50), truncate(hash, 12)}, "-"))
}

// toDNSLabel converts the string to a valid DNS label.
// source: crossplane/internal/xpkg/name.go
// nolint:gocyclo
func toDNSLabel(s string) string {
	var cut strings.Builder

	for i := range s {
		b := s[i]
		if ('a' <= b && b <= 'z') || ('0' <= b && b <= '9') {
			cut.WriteByte(b)
		}

		if (b == '.' || b == '/' || b == ':' || b == '-') && (i != 0 && i != 62 && i != len(s)-1) {
			cut.WriteByte('-')
		}

		if i == 62 {
			break
		}
	}

	return strings.Trim(cut.String(), "-")
}

func truncate(str string, num int) string {
	t := str
	if len(str) > num {
		t = str[0:num]
	}

	return t
}

// loadCrossplaneControllerImageToCluster loads the controller image into the oci cache of the given cluster
func loadCrossplaneControllerImageToCluster(clusterName string, image *string) env.Func {
	if image == nil {
		// no-op
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			return ctx, nil
		}
	}
	return envfuncs.LoadDockerImageToCluster(clusterName, *image)
}

// installCrossplaneProviderEnvFunc is an env.Func to install a crossplane provider into the given cluster
func installCrossplaneProviderEnvFunc(_ string, opts InstallCrossplaneProviderOptions) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		klog.V(4).Infof("Installing crossplane provider %s: %s", opts.Name, opts.Package)

		digest, err := retrieveDigest(ctx, opts.Package)
		if err != nil {
			return ctx, err
		}
		if digest == localImageDigest {
			opts.Package += localImageDigest
		}

		data := struct {
			Name             string
			Package          string
			ControllerConfig string
			RuntimeConfig    string
		}{
			Name:    opts.Name,
			Package: opts.Package,
		}

		if opts.ControllerConfig != nil {
			data.ControllerConfig = opts.Name

			err := applyControllerConfig(ctx, cfg, opts)
			if err != nil {
				return ctx, err
			}
		}
		if opts.DeploymentRuntimeConfig != nil {
			data.RuntimeConfig = opts.DeploymentRuntimeConfig.ObjectMeta.Name
			err := applyDeploymentRuntimeConfig(ctx, cfg, opts)
			if err != nil {
				return ctx, err
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

func applyControllerConfig(ctx context.Context, cfg *envconf.Config, opts InstallCrossplaneProviderOptions) error {
	config := opts.ControllerConfig.DeepCopy()
	config.TypeMeta.Kind = "ControllerConfig"
	config.TypeMeta.APIVersion = controllerConfigSchema.GroupVersion().Identifier()
	config.ObjectMeta = metav1.ObjectMeta{
		Name: opts.Name,
	}

	cl, err := dynamic.NewForConfig(cfg.Client().RESTConfig())
	if err != nil {
		return err
	}
	res := cl.Resource(controllerConfigSchema)
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config)
	if err != nil {
		return err
	}
	unstruc := unstructured.Unstructured{Object: data}
	_, err = res.Create(ctx, &unstruc, metav1.CreateOptions{})
	return err
}

func applyDeploymentRuntimeConfig(ctx context.Context, cfg *envconf.Config, opts InstallCrossplaneProviderOptions) error {
	klog.V(4).Info("Installing DeploymentRuntimeConfig")
	config := opts.DeploymentRuntimeConfig.DeepCopy()
	config.TypeMeta.Kind = "DeploymentRuntimeConfig"
	config.TypeMeta.APIVersion = deploymentRuntimeConfigSchema.GroupVersion().Identifier()

	cl, err := dynamic.NewForConfig(cfg.Client().RESTConfig())
	if err != nil {
		return err
	}
	res := cl.Resource(deploymentRuntimeConfigSchema)
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config)
	if err != nil {
		return err
	}
	unstruc := unstructured.Unstructured{Object: data}
	_, err = res.Create(ctx, &unstruc, metav1.CreateOptions{})
	if err != nil && apierrors.IsAlreadyExists(err) {
		// replace the config if it already exists
		obj, err := res.Get(ctx, unstruc.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		unstruc.SetResourceVersion(obj.GetResourceVersion())
		if _, err = res.Update(ctx, &unstruc, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return err
}

func awaitProviderHealthy(name string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		r, err := resources.New(cfg.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}
		err = wait.For(
			xpconditions.New(r).ProviderConditionMatch(
				name,
				"Healthy",
				corev1.ConditionTrue,
			), wait.WithTimeout(time.Minute*5),
		)
		return ctx, err
	}
}

// applyResources creates or replaces objects that already exist
func applyResources(ctx context.Context, cfg *envconf.Config, crs string) (context.Context, error) {
	r, err := resources.New(cfg.Client().RESTConfig())
	if err != nil {
		return ctx, err
	}

	if err := decoder.DecodeEach(ctx, strings.NewReader(crs), decoder.CreateHandler(r)); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ctx, decoder.DecodeEach(ctx, strings.NewReader(crs), resHelper.ReplaceHandler(r))
		}
		return ctx, err
	}
	return ctx, nil
}

// getClusterControlPlaneName returns the supposed name of the given clusters control plane
func getClusterControlPlaneName(clusterName string) string {
	return fmt.Sprintf("%s-control-plane", clusterName)
}

// Compose executes multiple env.Funcs in a row
func Compose(envfuncs ...env.Func) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		for _, envfunc := range envfuncs {
			var err error
			if ctx, err = envfunc(ctx, cfg); err != nil {
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

// SimpleSecret Create Opaque secret from non-binary data in crossplane namespace
func SimpleSecret(name string, namespace string, stringData map[string]string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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

// DumpLogs Dumps the logs of the cluster to `$PWD/logs` using kind export func
func DumpLogs(clusterName string, dir string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {

		cluster, ok := envfuncs.GetClusterFromContext(ctx, clusterName)
		if !ok {
			return ctx, errors.New(errNoClusterInContext)
		}
		cur, err := os.Getwd()
		if err != nil {
			return ctx, err
		}
		dest := path.Join(cur, "logs", dir)
		klog.Infof("Writing kind logs to %s", dest)

		err = cluster.ExportLogs(ctx, dest)
		return ctx, err
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

// CrossplaneOpt Option alias for configuring aspects of crossplane installation
type CrossplaneOpt = helm.Option

// Version configures the version of crossplane to be installed
func Version(version string) CrossplaneOpt {
	return func(opts *helm.Opts) {
		opts.Version = version
	}
}

// Registry configures the registry crossplane uses by adding it to the args values
func Registry(registry string) CrossplaneOpt {
	return func(opts *helm.Opts) {
		opts.Args = append(opts.Args, "--set", fmt.Sprintf("args={--registry=%s}", registry))
	}
}
