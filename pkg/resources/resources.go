package resources

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
	v1extensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/crossplane-contrib/xp-testing/pkg/xpconditions"
)

var (
	providerSchema = schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"}
)

// ImportResources gets the resources from dir
func ImportResources(ctx context.Context, t *testing.T, cfg *envconf.Config, dir string, decoderOptions ...decoder.DecodeOption) {
	r := resClient(cfg)

	r.WithNamespace(cfg.Namespace())

	if exists, err := checkAtLeastOneYamlFile(dir); err != nil {
		t.Fatal(err)
	} else if !exists {
		t.Fatalf("No yaml files found for %s", dir)
		return
	}
	decoderOptions = append(decoderOptions, decoder.MutateNamespace(cfg.Namespace()))
	// managed resources fare cluster scoped, so if we patched them with the test namespace it won't do anything
	errdecode := decoder.DecodeEachFile(
		ctx, os.DirFS(dir), "*",
		decoder.CreateIgnoreAlreadyExists(r),
		decoderOptions...,
	)
	if errdecode != nil {
		t.Fatal(errdecode)
	}
}

func resClient(cfg *envconf.Config) *resources.Resources {
	r, _ := GetResourcesWithRESTConfig(cfg)
	return r
}

// GetResourcesWithRESTConfig returns new resource from REST config
func GetResourcesWithRESTConfig(cfg *envconf.Config) (*resources.Resources, error) {
	r, err := resources.New(cfg.Client().RESTConfig())
	return r, err
}

func checkAtLeastOneYamlFile(dir string) (bool, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return false, err
	}

	return len(files) > 0, nil
}

// WaitForResourcesToBeSynced waits until all managed resources are synced and available
func WaitForResourcesToBeSynced(
	ctx context.Context,
	cfg *envconf.Config,
	dir string,
	opts ...wait.Option,
) error {
	objects, err := getObjectsToImport(ctx, cfg, dir)
	if err != nil {
		return err
	}

	klog.V(4).Infof("Waiting for all objects to become on the following objects\n %s", identifiers(objects))

	res := cfg.Client().Resources()

	err = wait.For(
		xpconditions.New(res).ManagedResourcesReadyAndReady(&mockList{Items: objects}), opts...,
	)
	return err
}

type mockList struct {
	metav1.ListInterface
	runtime.Object
	Items []k8s.Object
}

// Identifier returns k8s object name
func Identifier(object k8s.Object) string {
	return fmt.Sprintf("%s/%s", object.GetObjectKind().GroupVersionKind().String(), object.GetName())
}

func identifiers(objects []k8s.Object) string {
	val := ""
	for _, object := range objects {
		val = fmt.Sprintf("%s\n", Identifier(object))
	}
	return val
}

func getObjectsToImport(ctx context.Context, cfg *envconf.Config, dir string) ([]k8s.Object, error) {
	r := resClient(cfg)

	r.WithNamespace(cfg.Namespace())

	objects := make([]k8s.Object, 0)
	err := decoder.DecodeEachFile(
		ctx, os.DirFS(dir), "*",
		func(ctx context.Context, obj k8s.Object) error {
			objects = append(objects, obj)
			return nil
		},
		decoder.MutateNamespace(cfg.Namespace()),
	)
	return objects, err
}

// DumpManagedResources dumps resources with CRDs and Providers
func DumpManagedResources(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	resClient := resClient(cfg)
	dumpWithCRDs(ctx, t, cfg, resClient)
	dumpProviders(ctx, t, resClient)
	return ctx
}

func dumpProviders(ctx context.Context, t *testing.T, client *resources.Resources) {
	dynamiq := dynamic.NewForConfigOrDie(client.GetConfig())

	res := dynamiq.Resource(providerSchema)
	list, err := res.List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
		return
	}
	for _, provider := range list.Items {
		t.Log(provider)
	}
}

func dumpWithCRDs(ctx context.Context, t *testing.T, cfg *envconf.Config, client *resources.Resources) {
	var crds v1extensions.CustomResourceDefinitionList

	if err := v1extensions.AddToScheme(client.GetScheme()); err != nil {
		t.Fatal(err)
	}

	if err := client.List(ctx, &crds); err != nil {
		t.Fatal(err)
	}
	t.Log("Dumping all managed resources")
	var relevantCRDs []v1extensions.CustomResourceDefinition
	for _, crd := range crds.Items {
		if lo.Contains(crd.Spec.Names.Categories, "managed") {
			relevantCRDs = append(relevantCRDs, crd)
		}
	}
	dynamiq := dynamic.NewForConfigOrDie(cfg.Client().RESTConfig())
	for _, crd := range relevantCRDs {
		if crd.Spec.Scope == v1extensions.ClusterScoped {
			for _, version := range crd.Spec.Versions {
				dumpResourcesOfCRDs(ctx, t, dynamiq, crd, version)
			}
		} else {
			t.Logf("Skipped %s, since its not cluster scoped", crd.Spec.Names.Kind)
		}
	}
}

func getResourcesDynamically(
	ctx context.Context, dynamic dynamic.Interface,
	group string, version string, resource string,
) (
	[]unstructured.Unstructured, error,
) {

	resourceID := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}
	list, err := dynamic.Resource(resourceID).
		List(ctx, metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

func dumpResourcesOfCRDs(ctx context.Context, t *testing.T, dynamiq dynamic.Interface, crd v1extensions.CustomResourceDefinition, version v1extensions.CustomResourceDefinitionVersion) {
	resourcesList, err := getResourcesDynamically(
		ctx,
		dynamiq,
		crd.Spec.Group,
		version.Name,
		crd.Spec.Names.Plural,
	)

	if err != nil {
		t.Error(err)
	}
	for _, res := range resourcesList {
		marshal, err := yaml.Marshal(&res)
		if err != nil {
			t.Log(res)
		}
		t.Log(string(marshal))
	}
}

// DeleteResources deletes previously imported resources
func DeleteResources(ctx context.Context, t *testing.T, cfg *envconf.Config, manifestDir string, timeout wait.Option) context.Context {
	klog.V(4).Info("Attempt to delete previously imported resources")
	r, _ := GetResourcesWithRESTConfig(cfg)
	objects, err := getObjectsToImport(ctx, cfg, manifestDir)
	if err != nil {
		t.Fatal(objects)
	}
	if err = deleteObjects(ctx, cfg, manifestDir); err != nil && !errors.IsNotFound(err) {
		t.Fatal(err)
	}

	if err = wait.For(
		conditions.New(r).ResourcesDeleted(&mockList{Items: objects}),
		timeout,
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func deleteObjects(ctx context.Context, cfg *envconf.Config, dir string) error {
	r := resClient(cfg)
	r.WithNamespace(cfg.Namespace())

	return decoder.DecodeEachFile(
		ctx, os.DirFS(dir), "*",
		decoder.DeleteHandler(r),
		decoder.MutateNamespace(cfg.Namespace()),
	)
}

// AwaitResourceUpdateOrError waits for a given resource to update with a timeout of 3 minutes
func AwaitResourceUpdateOrError(ctx context.Context, t *testing.T, cfg *envconf.Config, object k8s.Object) {
	xpc := xpconditions.New(cfg.Client().Resources())
	AwaitResourceUpdateFor(
		ctx, t, cfg, object, xpc.IsManagedResourceReadyAndReady,
		wait.WithTimeout(time.Minute*3),
	)
}

// AwaitResourceUpdateFor waits for a given resource to be updated
func AwaitResourceUpdateFor(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	object k8s.Object,
	fn func(object k8s.Object) bool,
	opts ...wait.Option,
) {
	res := cfg.Client().Resources()

	err := res.Update(ctx, object)
	if err != nil {
		t.Fatal(err)
	}

	err = wait.For(
		conditions.New(res).ResourceMatch(object, fn), opts...,
	)
	if err != nil {
		t.Error(err)
	}
}

// AwaitResourceDeletionOrFail deletes a given k8s object with a timeout of 3 minutes
func AwaitResourceDeletionOrFail(ctx context.Context, t *testing.T, cfg *envconf.Config, object k8s.Object) {
	res := cfg.Client().Resources()

	err := res.Delete(ctx, object)
	if err != nil {
		t.Fatalf("Failed to delete object %s.", Identifier(object))
	}

	err = wait.For(conditions.New(res).ResourceDeleted(object), wait.WithTimeout(time.Minute*3))
	if err != nil {
		t.Fatalf(
			"Failed to delete object in time %s.",
			Identifier(object),
		)
	}
}

// ResourceTestConfig is a test configuration for a resource.
// It contains the kind of resource and the object to be tested
// and then provides basic CRD tests for the resource.
type ResourceTestConfig struct {
	Kind              string
	Obj               *k8s.Object
	AdditionalSteps   map[string]func(context.Context, *testing.T, *envconf.Config) context.Context
	ResourceDirectory string
}

// NewResourceTestConfig constructs a simple version of ResourceTestConfig
func NewResourceTestConfig(obj *k8s.Object, kind string) *ResourceTestConfig {
	return &ResourceTestConfig{Kind: kind, Obj: obj, AdditionalSteps: nil, ResourceDirectory: DefaultCRFolder(kind)}

}

// DefaultCRFolder returns a relative path to a folder where CR's for tests are suspected
func (r *ResourceTestConfig) DefaultCRFolder() string {
	return DefaultCRFolder(r.Kind)
}

// DefaultCRFolder returns a relative path to a folder where CR's for tests are suspected
func DefaultCRFolder(kind string) string {
	return path.Join("./crs", kind)
}

// Setup creates the resource in the cluster.
func (r *ResourceTestConfig) Setup(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	t.Logf("Apply %s", r.Kind)
	ImportResources(ctx, t, cfg, r.ResourceDirectory)

	return ctx
}

// Teardown does nothing for now but exists here for completeness.
func (r *ResourceTestConfig) Teardown(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return ctx
}

// AssessCreate checks that the resource was created successfully.
func (r *ResourceTestConfig) AssessCreate(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	if err := WaitForResourcesToBeSynced(ctx, cfg, r.ResourceDirectory, wait.WithTimeout(time.Minute*5)); err != nil {
		DumpManagedResources(ctx, t, cfg)
		t.Fatal(err)
	}
	return ctx
}

// AssessUpdate does nothing for now but exists here for completeness.
func (r *ResourceTestConfig) AssessUpdate(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return ctx
}

// AssessDelete checks that the resource was deleted successfully.
func (r *ResourceTestConfig) AssessDelete(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return DeleteResources(ctx, t, cfg, r.ResourceDirectory, wait.WithTimeout(time.Minute*5))
}
