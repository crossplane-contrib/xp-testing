// Package mutate provides reusable edits for test CRs, applied at decode time.
//
// Each helper returns a decoder.MutateFunc, so it can be used with decoder.MutateOption(...)
// anywhere, e.g. in resources.ResourceTestConfig.DecoderOptions.
//
// Suffixes are not truncated: callers must keep names within API limits
// (e.g. 63 characters for label values) and provider-specific limits on external names.
package mutate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
)

var runIDEnvVars = []string{"E2E_RUN_ID", "GITHUB_RUN_ID"}

var runID = sync.OnceValue(func() string { return lookupRunID(os.Getenv) })

func lookupRunID(getenv func(string) string) string {
	for _, env := range runIDEnvVars {
		if v := getenv(env); v != "" {
			return strings.ToLower(v)
		}
	}
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RunID returns a short identifier for this test run, stable for the lifetime
// of the test binary: E2E_RUN_ID, then GITHUB_RUN_ID, else 6 random hex chars.
func RunID() string { return runID() }

// ForKinds applies fn only to objects of the given kinds.
func ForKinds(kinds []string, fn decoder.MutateFunc) decoder.MutateFunc {
	return func(obj k8s.Object) error {
		if !slices.Contains(kinds, obj.GetObjectKind().GroupVersionKind().Kind) {
			return nil
		}
		return fn(obj)
	}
}

// NameSuffix appends "-<suffix>" to metadata.name.
func NameSuffix(suffix string) decoder.MutateFunc {
	return func(obj k8s.Object) error {
		obj.SetName(obj.GetName() + "-" + suffix)
		return nil
	}
}

// FieldSuffix appends "-<suffix>" to the string field at path, e.g.
// FieldSuffix(id, "spec", "forProvider", "name"). Missing or empty fields are ignored.
func FieldSuffix(suffix string, path ...string) decoder.MutateFunc {
	return asUnstructured(func(u *unstructured.Unstructured) error {
		v, found, err := unstructured.NestedString(u.Object, path...)
		if err != nil || !found || v == "" {
			return err
		}
		return unstructured.SetNestedField(u.Object, v+"-"+suffix, path...)
	})
}

// RefSuffix appends "-<suffix>" to the name of every reference under spec
// (fields ending in "Ref", and items of fields ending in "Refs"), so references
// keep pointing at objects renamed by NameSuffix. providerConfigRef is always
// skipped; also pass references to objects not created from the test directory
// (e.g. pre-existing secrets) in skip.
func RefSuffix(suffix string, skip ...string) decoder.MutateFunc {
	skip = append(slices.Clone(skip), "providerConfigRef")
	return asUnstructured(func(u *unstructured.Unstructured) error {
		if spec, ok := u.Object["spec"].(map[string]any); ok {
			walkRefs(spec, skip, func(k string, ref map[string]any) {
				if strings.HasSuffix(k, "Ref") || strings.HasSuffix(k, "Refs") {
					if n, ok := ref["name"].(string); ok && n != "" {
						ref["name"] = n + "-" + suffix
					}
				}
			})
		}
		return nil
	})
}

// ToClusterScoped turns namespaced managed resources into their cluster-scoped
// equivalent. groups maps namespaced API groups to cluster API groups; objects
// of other groups (e.g. Secrets) are left alone. It
//   - rewrites apiVersion and clears metadata.namespace,
//   - drops spec.providerConfigRef.kind (the cluster-scoped reference has no kind),
//   - sets secretNamespace on secret references that have none (the cluster-scoped
//     SecretKeySelector/SecretReference require a namespace).
//
// The object must be *unstructured.Unstructured: a typed namespaced object cannot
// carry the cluster-scoped GVK or the secret ref namespaces.
func ToClusterScoped(groups map[string]string, secretNamespace string) decoder.MutateFunc {
	return func(obj k8s.Object) error {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("ToClusterScoped needs *unstructured.Unstructured, got %T", obj)
		}
		gvk := u.GroupVersionKind()
		clusterGroup, ok := groups[gvk.Group]
		if !ok {
			return nil
		}
		gvk.Group = clusterGroup
		u.SetGroupVersionKind(gvk)
		u.SetNamespace("")
		unstructured.RemoveNestedField(u.Object, "spec", "providerConfigRef", "kind")
		if spec, ok := u.Object["spec"].(map[string]any); ok {
			walkRefs(spec, nil, func(k string, ref map[string]any) {
				if isSecretRef(k) {
					if _, ok := ref["namespace"]; !ok {
						ref["namespace"] = secretNamespace
					}
				}
			})
		}
		return nil
	}
}

// ToNamespaced turns cluster-scoped managed resources into their namespaced
// equivalent, the reverse of ToClusterScoped. groups maps cluster API groups to
// namespaced API groups; objects of other groups are left alone. It
//   - rewrites apiVersion and sets metadata.namespace,
//   - sets spec.providerConfigRef.kind to providerConfigKind if it has none,
//   - removes namespace from secret references (namespaced secret references
//     always point into the namespace of the resource).
//
// The object must be *unstructured.Unstructured, see ToClusterScoped.
func ToNamespaced(groups map[string]string, namespace, providerConfigKind string) decoder.MutateFunc {
	return func(obj k8s.Object) error {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("ToNamespaced needs *unstructured.Unstructured, got %T", obj)
		}
		gvk := u.GroupVersionKind()
		namespacedGroup, ok := groups[gvk.Group]
		if !ok {
			return nil
		}
		gvk.Group = namespacedGroup
		u.SetGroupVersionKind(gvk)
		u.SetNamespace(namespace)
		if pcRef, ok, _ := unstructured.NestedMap(u.Object, "spec", "providerConfigRef"); ok {
			if _, hasKind := pcRef["kind"]; !hasKind {
				if err := unstructured.SetNestedField(u.Object, providerConfigKind, "spec", "providerConfigRef", "kind"); err != nil {
					return err
				}
			}
		}
		if spec, ok := u.Object["spec"].(map[string]any); ok {
			walkRefs(spec, nil, func(k string, ref map[string]any) {
				if isSecretRef(k) {
					delete(ref, "namespace")
				}
			})
		}
		return nil
	}
}

func isSecretRef(key string) bool {
	return strings.HasSuffix(key, "SecretRef") || key == "writeConnectionSecretToRef"
}

// walkRefs calls fn for every map (or list item) under m, keyed by its field
// name, skipping fields in skip and their subtrees.
func walkRefs(m map[string]any, skip []string, fn func(key string, ref map[string]any)) {
	for k, v := range m {
		if slices.Contains(skip, k) {
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			fn(k, val)
			walkRefs(val, skip, fn)
		case []any:
			for _, item := range val {
				if im, ok := item.(map[string]any); ok {
					fn(k, im)
					walkRefs(im, skip, fn)
				}
			}
		}
	}
}

// asUnstructured runs fn on obj as unstructured, converting typed objects
// to and from unstructured.
func asUnstructured(fn func(*unstructured.Unstructured) error) decoder.MutateFunc {
	return func(obj k8s.Object) error {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			return fn(u)
		}
		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return err
		}
		u := &unstructured.Unstructured{Object: m}
		if err := fn(u); err != nil {
			return err
		}
		return runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	}
}
