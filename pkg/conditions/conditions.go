package conditions

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
	corev1 "k8s.io/api/core/v1"
	apimachinerywait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	apimachineryconditions "sigs.k8s.io/e2e-framework/klient/wait/conditions"
)

// Conditions helps with matching resources on conditions
type Conditions struct {
	apimachineryconditions.Condition
	resources *resources.Resources
}

// New is constructor for Conditions
func New(r *resources.Resources) *Conditions {
	return &Conditions{Condition: *apimachineryconditions.New(r), resources: r}
}

// ManagedResourceConditionMatch checks if a ManagedResource has a matching condition
func (c *Conditions) ManagedResourceConditionMatch(
	provider k8s.Object,
	conditionType xpv1.ConditionType,
	conditionState corev1.ConditionStatus,
) apimachinerywait.ConditionFunc {
	return func() (done bool, err error) {
		klog.V(4).Infof("Awaiting provider %s to be ready", provider.GetName())
		if err := c.resources.Get(context.TODO(), provider.GetName(), provider.GetNamespace(), provider); err != nil {
			return false, err
		}
		for _, cond := range provider.(*pkgv1.Provider).Status.Conditions {
			klog.V(4).Infof("provider %s, condition: %s: %s", provider.GetName(), cond.Type, cond.Status)
			if cond.Type == conditionType && cond.Status == conditionState {
				done = true
			}
		}
		return
	}
}
