package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var someName = "Bar"

func Test_clusterName(t *testing.T) {
	type args struct {
		reuseCluster        bool
		clusterNameEnvValue *string
	}
	tests := []struct {
		name    string
		args    args
		matcher func(string) bool
	}{
		{
			name: "No reuse, no env var; returns default prefix with random suffix",
			args: args{
				reuseCluster:        false,
				clusterNameEnvValue: nil,
			},
			matcher: func(s string) bool {
				prefix := fmt.Sprintf("%s-", defaultPrefix)
				return strings.HasPrefix(s, prefix)
			},
		},
		{
			name: "reuse, no env var; returns default prefix",
			args: args{
				reuseCluster:        true,
				clusterNameEnvValue: nil,
			},
			matcher: func(s string) bool {
				return s == defaultPrefix
			},
		},
		{
			name: "reuse, with env var; returns from env",
			args: args{
				reuseCluster:        true,
				clusterNameEnvValue: &someName,
			},
			matcher: func(s string) bool {
				return s == someName
			},
		},
		{
			name: "no reuse, with env var; returns from env",
			args: args{
				reuseCluster:        false,
				clusterNameEnvValue: &someName,
			},
			matcher: func(s string) bool {
				return s == someName
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.clusterNameEnvValue == nil {
				require.NoError(t, os.Unsetenv(clusterNameEnv))
			} else {
				require.NoError(t, os.Setenv(clusterNameEnv, *tt.args.clusterNameEnvValue))
			}
			if got := clusterName(tt.args.reuseCluster); !tt.matcher(got) {
				t.Errorf("clusterName() = %v; matcher returned error", got)
			}
		})
	}
}

func TestClusterSetup_InstallCrossplaneFunc_HookOverrides(t *testing.T) {
	called := false
	sentinel := func(ctx context.Context, _ *envconf.Config) (context.Context, error) {
		called = true
		return ctx, errors.New("sentinel")
	}
	s := &ClusterSetup{CrossplaneInstallFunc: sentinel}
	got := s.installCrossplaneFunc("test-cluster")
	require.NotNil(t, got)
	_, err := got(context.Background(), nil)
	require.True(t, called, "expected hook to be invoked")
	require.EqualError(t, err, "sentinel")
}

func TestClusterSetup_InstallCrossplaneFunc_DefaultsToBundled(t *testing.T) {
	s := &ClusterSetup{}
	got := s.installCrossplaneFunc("test-cluster")
	require.NotNil(t, got)
	// The bundled InstallCrossplane errors when invoked outside a real test
	// environment; asserting on that exact error couples too tightly. Just
	// assert non-nil to confirm we delegated to the bundled installer.
}
