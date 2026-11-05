package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/crossplane-contrib/xp-testing/pkg/mutate"
)

const testCR = `apiVersion: nop.crossplane.io/v1alpha1
kind: NopResource
metadata:
  name: example
`

func Test_getObjectsToImport(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cr.yaml"), []byte(testCR), 0o600))
	cfg := envconf.New().WithNamespace("ns")

	tests := []struct {
		name          string
		opts          []decoder.DecodeOption
		wantName      string
		wantNamespace string
	}{
		{name: "defaults", wantName: "example", wantNamespace: "ns"},
		{
			name:          "caller options run after namespace mutation",
			opts:          []decoder.DecodeOption{decoder.MutateNamespace("other"), decoder.MutateOption(mutate.NameSuffix("x1"))},
			wantName:      "example-x1",
			wantNamespace: "other",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects, err := getObjectsToImport(context.Background(), cfg, []string{dir}, tt.opts...)
			require.NoError(t, err)
			require.Len(t, objects, 1)
			assert.Equal(t, tt.wantName, objects[0].GetName())
			assert.Equal(t, tt.wantNamespace, objects[0].GetNamespace())
		})
	}
}
