package logging

import (
	"flag"
	"fmt"

	"k8s.io/klog/v2"
)

const (
	verbosityLevel = "v"
	logToStdErr    = "alsologtostderr"
)

// EnableVerboseLogging enables klog in verbosity of 4 if not overwritten by verbosity argument
func EnableVerboseLogging(verbosity *int) {
	defaultVerbosity := 4
	effectiveVerbosity := defaultVerbosity
	if verbosity != nil {
		effectiveVerbosity = *verbosity
	}
	if flag.Lookup(verbosityLevel) == nil || flag.Lookup(logToStdErr) == nil {
		klog.InitFlags(nil)
	}

	if err := flag.Set(logToStdErr, fmt.Sprintf("%t", true)); err != nil {
		panic(err)
	}

	if err := flag.Set(verbosityLevel, fmt.Sprintf("%d", effectiveVerbosity)); err != nil {
		panic(err)
	}

	defer klog.Flush()
	flag.Parse()
}
