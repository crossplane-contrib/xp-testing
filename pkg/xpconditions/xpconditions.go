package xpconditions

import (
	"context"
	"encoding/json"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimachinerywait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	apimachineryconditions "sigs.k8s.io/e2e-framework/klient/wait/conditions"
)

var (
	providerSchema = schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1", Resource: "provider"}
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

		cl, err := dynamic.NewForConfig(c.resources.GetConfig())
		if err != nil {
			return false, err
		}
		res := cl.Resource(providerSchema)
		providerObject, err := res.Get(ctx, provider.GetName(), metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		result := awaitCondition(providerObject, conditionType, conditionStatus)
		return result, nil
	}
}

func awaitCondition(unstrOb *unstructured.Unstructured, desiredType xpv1.ConditionType, desiredStatus corev1.ConditionStatus) bool {

	statusObj, ok := unstrOb.Object["status"].(map[string]interface{})

	if statusObj == nil || !ok {
		klog.V(4).Infof("Object (%s) %s has no status", unstrOb.GroupVersionKind().String(), unstrOb.GetName())
		return false
	}

	conditions, ok := statusObj["conditions"].([]interface{})
	if conditions == nil || !ok {
		klog.V(4).Infof("Object (%s) %s has no conditions", unstrOb.GroupVersionKind().String(), unstrOb.GetName())
		return false
	}

	status := ""
	for _, condition := range conditions {
		c := condition.(map[string]interface{})
		if c["type"] == string(desiredType) {
			status = c["status"].(string)
		}
	}
	matchedConditionStatus := false
	if status == string(desiredStatus) {
		matchedConditionStatus = true
	}

	klog.V(4).Infof("Object (%s) %s, condition: %s: %s", unstrOb.GroupVersionKind().String(), unstrOb.GetName(), desiredType, matchedConditionStatus)

	return matchedConditionStatus
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
