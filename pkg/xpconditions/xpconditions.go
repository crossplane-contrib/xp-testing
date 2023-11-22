package xpconditions

import (
	"context"
	"encoding/json"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// ProviderConditionMatch checks if a Provider has a matching condition
func (c *Conditions) ProviderConditionMatch(
	provider k8s.Object,
	conditionType xpv1.ConditionType,
	conditionStatus corev1.ConditionStatus,
) apimachinerywait.ConditionWithContextFunc {
	return func(ctx context.Context) (done bool, err error) {
		klog.V(4).Infof("Awaiting provider %s to be ready", provider.GetName())

		if err := c.resources.Get(ctx, provider.GetName(), provider.GetNamespace(), provider); err != nil {
			return false, err
		}
		p := provider.(*pkgv1.Provider)

		res := awaitCondition(*p, conditionType, conditionStatus)
		return res, nil
	}
}

func awaitCondition(provider pkgv1.Provider, conditionType xpv1.ConditionType, conditionStatus corev1.ConditionStatus) bool {
	for _, cond := range provider.Status.Conditions {
		klog.V(4).Infof("provider %s, condition: %s: %s", provider.GetName(), cond.Type, cond.Status)
		if cond.Type == conditionType && cond.Status == conditionStatus {
			return true
		}
	}
	return false
}

// IsManagedResourceReadyAndReady returns if a managed resource has condtions Synced = True and Ready = True
func (c *Conditions) IsManagedResourceReadyAndReady(object k8s.Object) bool {

	managed := convertToManaged(object)
	return managedCheckCondition(managed, xpv1.TypeSynced, corev1.ConditionTrue) &&
		managedCheckCondition(managed, xpv1.TypeReady, corev1.ConditionTrue)
}

func convertToManaged(object k8s.Object) resource.Managed {
	unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil
	}
	var managed DummyManaged
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructured, &managed)

	if err != nil {
		panic(err)
	}

	return &managed
}

// ManagedResourcesReadyAndReady checks if a list of ManagedResources has a matching condition
func (c *Conditions) ManagedResourcesReadyAndReady(
	list k8s.ObjectList,
) apimachinerywait.ConditionWithContextFunc {
	return c.ResourcesMatch(list, c.IsManagedResourceReadyAndReady)
}

func managedCheckCondition(o resource.Managed, conditionType xpv1.ConditionType, want corev1.ConditionStatus) bool {
	klog.V(4).Infof("Checking Managed Resource %s to be condition: %s: %s", o.GetName(), conditionType, want)
	got := o.GetCondition(conditionType)
	klog.V(4).Infof("Got Managed Resource %s to be condition: %s: %s, message: %s", o.GetName(), conditionType, got.Status, got.Message)
	return want == got.Status
}

var _ resource.Managed = &DummyManaged{}

// DummyManaged acts as a fake / dummy to allow generic checks on any managed resource
type DummyManaged struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	resource.ProviderConfigReferencer
	resource.ConnectionSecretWriterTo
	resource.ConnectionDetailsPublisherTo
	resource.Manageable
	resource.Orphanable
	xpv1.ConditionedStatus `json:"status"`
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (m *DummyManaged) DeepCopyObject() runtime.Object {
	out := &DummyManaged{}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}
