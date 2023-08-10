package logging

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnableVerboseLogging(t *testing.T) {
	t.Run(
		"default verbosity", func(t *testing.T) {
			resetFlags()
			EnableVerboseLogging(nil)

			verbseFlag := flag.Lookup("v")
			assert.Equal(t, verbseFlag.Value.String(), "4")
		},
	)
	t.Run(
		"verbosity overwritten", func(t *testing.T) {
			resetFlags()

			EnableVerboseLogging(ptr(10))

			verbseFlag := flag.Lookup("v")
			assert.Equal(t, verbseFlag.Value.String(), "10")
		},
	)
}

func resetFlags() {
	flag.CommandLine = flag.NewFlagSet("", flag.ContinueOnError)
}

func ptr[T any](v T) *T {
	return &v
}
