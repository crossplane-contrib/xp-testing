package logging

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnableVerboseLogging(t *testing.T) {
	t.Run(
		"default verbosity", func(t *testing.T) {
			resetFlags()
			EnableVerboseLogging(nil)

			verbseFlag := flag.Lookup("v")
			require.Equal(t, "4", verbseFlag.Value.String())
		},
	)
	t.Run(
		"verbosity overwritten", func(t *testing.T) {
			resetFlags()

			EnableVerboseLogging(ptr(10))

			verbseFlag := flag.Lookup("v")
			require.Equal(t, "10", verbseFlag.Value.String())
		},
	)
}

func resetFlags() {
	flag.CommandLine = flag.NewFlagSet("", flag.ContinueOnError)
}

func ptr[T any](v T) *T {
	return &v
}
