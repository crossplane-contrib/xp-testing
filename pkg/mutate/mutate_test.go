//nolint:goconst // map literals mirror YAML manifests
package mutate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/e2e-framework/klient/k8s"
)

func managed(apiVersion string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       "Thing",
		"metadata":   map[string]any{"name": "example"},
		"spec":       spec,
	}}
}

func TestRunID(t *testing.T) {
	first := RunID()
	assert.Equal(t, first, RunID())

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "E2E_RUN_ID wins", env: map[string]string{"E2E_RUN_ID": "ABC", "GITHUB_RUN_ID": "123"}, want: "abc"},
		{name: "GITHUB_RUN_ID fallback", env: map[string]string{"GITHUB_RUN_ID": "123"}, want: "123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, lookupRunID(func(k string) string { return tt.env[k] }))
		})
	}

	t.Run("random fallback", func(t *testing.T) {
		assert.Regexp(t, "^[0-9a-f]{6}$", lookupRunID(func(string) string { return "" }))
	})
}

func TestNameSuffix(t *testing.T) {
	tests := []struct {
		name string
		obj  k8s.Object
	}{
		{name: "typed", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example"}}},
		{name: "unstructured", obj: managed("example.crossplane.io/v1", nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, NameSuffix("x1")(tt.obj))
			assert.Equal(t, "example-x1", tt.obj.GetName())
		})
	}
}

func TestFieldSuffix(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		want    map[string]any
		wantErr bool
	}{
		{
			name: "present",
			spec: map[string]any{"forProvider": map[string]any{"name": "ext"}},
			want: map[string]any{"forProvider": map[string]any{"name": "ext-x1"}},
		},
		{
			name: "missing",
			spec: map[string]any{"forProvider": map[string]any{}},
			want: map[string]any{"forProvider": map[string]any{}},
		},
		{
			name: "empty",
			spec: map[string]any{"forProvider": map[string]any{"name": ""}},
			want: map[string]any{"forProvider": map[string]any{"name": ""}},
		},
		{
			name:    "wrong type",
			spec:    map[string]any{"forProvider": map[string]any{"name": int64(1)}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := managed("example.crossplane.io/v1", tt.spec)
			err := FieldSuffix("x1", "spec", "forProvider", "name")(u)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, u.Object["spec"])
		})
	}

	t.Run("typed", func(t *testing.T) {
		s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example"}, StringData: map[string]string{"k": "v"}}
		require.NoError(t, FieldSuffix("x1", "stringData", "k")(s))
		assert.Equal(t, "v-x1", s.StringData["k"])
	})
}

func TestRefSuffix(t *testing.T) {
	u := managed("example.crossplane.io/v1", map[string]any{
		"forProvider": map[string]any{
			"parentRef":   map[string]any{"name": "parent"},
			"memberRefs":  []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}},
			"externalRef": map[string]any{"name": "external"},
			"nested":      map[string]any{"deepRef": map[string]any{"name": "deep"}},
			"emptyRef":    map[string]any{"name": ""},
			"notRefField": map[string]any{"name": "plain"},
		},
		"providerConfigRef":          map[string]any{"name": "default"},
		"writeConnectionSecretToRef": map[string]any{"name": "conn"},
	})
	u.Object["status"] = map[string]any{"someRef": map[string]any{"name": "status"}}

	require.NoError(t, RefSuffix("x1", "externalRef")(u))

	fp := u.Object["spec"].(map[string]any)["forProvider"].(map[string]any)
	assert.Equal(t, "parent-x1", fp["parentRef"].(map[string]any)["name"])
	assert.Equal(t, "a-x1", fp["memberRefs"].([]any)[0].(map[string]any)["name"])
	assert.Equal(t, "b-x1", fp["memberRefs"].([]any)[1].(map[string]any)["name"])
	assert.Equal(t, "external", fp["externalRef"].(map[string]any)["name"])
	assert.Equal(t, "deep-x1", fp["nested"].(map[string]any)["deepRef"].(map[string]any)["name"])
	assert.Empty(t, fp["emptyRef"].(map[string]any)["name"])
	assert.Equal(t, "plain", fp["notRefField"].(map[string]any)["name"])

	spec := u.Object["spec"].(map[string]any)
	assert.Equal(t, "default", spec["providerConfigRef"].(map[string]any)["name"])
	assert.Equal(t, "conn-x1", spec["writeConnectionSecretToRef"].(map[string]any)["name"])
	assert.Equal(t, "status", u.Object["status"].(map[string]any)["someRef"].(map[string]any)["name"])
}

func TestForKinds(t *testing.T) {
	fn := ForKinds([]string{"Thing"}, NameSuffix("x1"))

	u := managed("example.crossplane.io/v1", nil)
	require.NoError(t, fn(u))
	assert.Equal(t, "example-x1", u.GetName())

	other := managed("example.crossplane.io/v1", nil)
	other.SetKind("Other")
	require.NoError(t, fn(other))
	assert.Equal(t, "example", other.GetName())
}

var (
	toCluster    = map[string]string{"example.m.crossplane.io": "example.crossplane.io"}
	toNamespaced = map[string]string{"example.crossplane.io": "example.m.crossplane.io"}
)

func namespacedThing() *unstructured.Unstructured {
	u := managed("example.m.crossplane.io/v1", map[string]any{
		"forProvider": map[string]any{
			"passwordSecretRef": map[string]any{"name": "pw", "key": "password"},
			"otherSecretRef":    map[string]any{"name": "other", "namespace": "keep-me"},
		},
		"providerConfigRef":          map[string]any{"name": "default", "kind": "ProviderConfig"},
		"writeConnectionSecretToRef": map[string]any{"name": "conn"},
	})
	u.SetNamespace("test-ns")
	return u
}

func TestToClusterScoped(t *testing.T) {
	t.Run("mapped group", func(t *testing.T) {
		u := namespacedThing()
		require.NoError(t, ToClusterScoped(toCluster, "crossplane-system")(u))

		assert.Equal(t, "example.crossplane.io/v1", u.GetAPIVersion())
		assert.Empty(t, u.GetNamespace())
		assert.Equal(t, map[string]any{"name": "default"}, u.Object["spec"].(map[string]any)["providerConfigRef"])
		fp := u.Object["spec"].(map[string]any)["forProvider"].(map[string]any)
		assert.Equal(t, "crossplane-system", fp["passwordSecretRef"].(map[string]any)["namespace"])
		assert.Equal(t, "keep-me", fp["otherSecretRef"].(map[string]any)["namespace"])
		assert.Equal(t, "crossplane-system",
			u.Object["spec"].(map[string]any)["writeConnectionSecretToRef"].(map[string]any)["namespace"])
	})

	t.Run("unmapped group", func(t *testing.T) {
		u := namespacedThing()
		u.SetAPIVersion("v1")
		want := u.DeepCopy()
		require.NoError(t, ToClusterScoped(toCluster, "crossplane-system")(u))
		assert.Equal(t, want, u)
	})

	t.Run("typed object", func(t *testing.T) {
		assert.Error(t, ToClusterScoped(toCluster, "crossplane-system")(&corev1.Secret{}))
	})
}

func TestToNamespaced(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		u := managed("example.crossplane.io/v1", map[string]any{
			"forProvider": map[string]any{
				"passwordSecretRef": map[string]any{"name": "pw", "namespace": "crossplane-system"},
			},
			"providerConfigRef":          map[string]any{"name": "default"},
			"writeConnectionSecretToRef": map[string]any{"name": "conn", "namespace": "crossplane-system"},
		})
		want := u.DeepCopy()

		require.NoError(t, ToNamespaced(toNamespaced, "test-ns", "ClusterProviderConfig")(u))
		assert.Equal(t, "example.m.crossplane.io/v1", u.GetAPIVersion())
		assert.Equal(t, "test-ns", u.GetNamespace())
		spec := u.Object["spec"].(map[string]any)
		assert.Equal(t, map[string]any{"name": "default", "kind": "ClusterProviderConfig"}, spec["providerConfigRef"])
		assert.Equal(t, map[string]any{"name": "conn"}, spec["writeConnectionSecretToRef"])
		assert.Equal(t, map[string]any{"name": "pw"}, spec["forProvider"].(map[string]any)["passwordSecretRef"])

		require.NoError(t, ToClusterScoped(toCluster, "crossplane-system")(u))
		assert.Equal(t, want, u)
	})

	t.Run("existing kind kept", func(t *testing.T) {
		u := managed("example.crossplane.io/v1", map[string]any{
			"providerConfigRef": map[string]any{"name": "default", "kind": "ProviderConfig"},
		})
		require.NoError(t, ToNamespaced(toNamespaced, "test-ns", "ClusterProviderConfig")(u))
		assert.Equal(t, "ProviderConfig", u.Object["spec"].(map[string]any)["providerConfigRef"].(map[string]any)["kind"])
	})

	t.Run("no providerConfigRef", func(t *testing.T) {
		u := managed("example.crossplane.io/v1", map[string]any{})
		require.NoError(t, ToNamespaced(toNamespaced, "test-ns", "ClusterProviderConfig")(u))
		assert.NotContains(t, u.Object["spec"], "providerConfigRef")
	})

	t.Run("unmapped group", func(t *testing.T) {
		u := managed("v1", map[string]any{})
		want := u.DeepCopy()
		require.NoError(t, ToNamespaced(toNamespaced, "test-ns", "ClusterProviderConfig")(u))
		assert.Equal(t, want, u)
	})

	t.Run("typed object", func(t *testing.T) {
		assert.Error(t, ToNamespaced(toNamespaced, "test-ns", "ClusterProviderConfig")(&corev1.Secret{}))
	})
}
