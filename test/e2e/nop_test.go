//go:build e2e

package e2e

import (
	"fmt"
	"testing"

	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/crossplane-contrib/xp-testing/pkg/mutate"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
)

func Test_Nop_v1alpha1(t *testing.T) {

	resource := resources.NewResourceTestConfig(nil, "Nop")

	fB := features.New(fmt.Sprintf("%v", resource.Kind))
	fB.WithLabel("kind", resource.Kind)
	fB.Setup(resource.Setup)
	fB.Assess("create", resource.AssessCreate)
	fB.Assess("delete", resource.AssessDelete)

	testenv.Test(t, fB.Feature())

}

// Test_Nop_Suffixed_v1alpha1 applies the same CRs as Test_Nop_v1alpha1 with a run ID suffix;
// create, wait and delete all have to use the mutated names for it to pass.
func Test_Nop_Suffixed_v1alpha1(t *testing.T) {

	id := mutate.RunID()
	resource := resources.NewResourceTestConfig(nil, "Nop")
	resource.DecoderOptions = []decoder.DecodeOption{
		decoder.MutateOption(mutate.NameSuffix(id)),
		decoder.MutateOption(mutate.RefSuffix(id)),
	}

	fB := features.New(fmt.Sprintf("%v-suffixed", resource.Kind))
	fB.WithLabel("kind", resource.Kind)
	fB.Setup(resource.Setup)
	fB.Assess("create", resource.AssessCreate)
	fB.Assess("delete", resource.AssessDelete)

	testenv.Test(t, fB.Feature())

}
