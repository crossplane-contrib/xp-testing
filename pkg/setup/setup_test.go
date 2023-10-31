package setup

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
