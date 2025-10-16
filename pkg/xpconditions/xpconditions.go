package xpconditions

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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
	providerSchema = schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"}
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
	name string,
	conditionType string,
	conditionStatus corev1.ConditionStatus,
) apimachinerywait.ConditionWithContextFunc {
	return func(ctx context.Context) (done bool, err error) {
		klog.V(4).Infof("Awaiting provider %s to be ready", name)

		cl, err := dynamic.NewForConfig(c.resources.GetConfig())
		if err != nil {
			return false, err
		}
		res := cl.Resource(providerSchema)
		providerObject, err := res.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, ignoreNotFound(err)
		}

		result := checkCondition(providerObject, conditionType, conditionStatus)
		return result, nil
	}
}

func ignoreNotFound(err error) error {
	if kerrors.IsNotFound(err) {
		return nil
	}
	return err
}

func checkCondition(unstruc *unstructured.Unstructured, desiredType string, desiredStatus corev1.ConditionStatus) bool {
	return checkConditionWithReason(unstruc, desiredType, desiredStatus, "")
}

func checkConditionWithReason(unstruc *unstructured.Unstructured, desiredType string, desiredStatus corev1.ConditionStatus, desiredReason string) bool {

	conditions, ok, err := unstructured.NestedSlice(unstruc.UnstructuredContent(), "status", "conditions")
	if err != nil {
		klog.V(4).Infof("Could not extract conditions of (%s) %s, %s", unstruc.GroupVersionKind().String(), unstruc.GetName(), err.Error())
		return false
	} else if !ok {
		klog.V(4).Infof("Object (%s) %s doesnt have conditions", unstruc.GroupVersionKind().String(), unstruc.GetName())
		return false
	}

	status := ""
	message := ""
	reason := ""
	for _, condition := range conditions {
		c := condition.(map[string]interface{})
		curType := c["type"]
		if curType == desiredType {
			status = c["status"].(string)
			msg, convertible := c["message"].(string)
			if convertible {
				message = msg
			}
			if c["reason"] != nil {
				reason = c["reason"].(string)
			}
		}
	}
	matchedConditionStatus := false
	if status == string(desiredStatus) {
		matchedConditionStatus = true
	}
	matchedConditionReason := true
	if desiredReason != "" {
		matchedConditionReason = reason == desiredReason
	}

	klog.V(4).Infof("Object (%s) %s, condition: %s: %s, matched: %t, message: %s, reason: %s",
		unstruc.GroupVersionKind().String(),
		unstruc.GetName(),
		desiredType,
		status,
		matchedConditionStatus,
		message,
		reason,
	)

	return matchedConditionStatus && matchedConditionReason
}

// IsManagedResourceReadyAndReady returns if a managed resource has condtions Synced = True and Ready = True
func (c *Conditions) IsManagedResourceReadyAndReady(object k8s.Object) bool {

	us := convertToUnstructured(object)
	return checkCondition(us, "Synced", corev1.ConditionTrue) &&
		checkCondition(us, "Ready", corev1.ConditionTrue)
}

func (c *Conditions) IsPaused(object k8s.Object) bool {
	obj := convertToUnstructured(object)
	return checkConditionWithReason(obj, "Synced", corev1.ConditionFalse, "ReconcilePaused")
}

func convertToUnstructured(object k8s.Object) *unstructured.Unstructured {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: data}
}

// ManagedResourcesReadyAndReady checks if a list of ManagedResources has a matching condition
func (c *Conditions) ManagedResourcesReadyAndReady(
	list k8s.ObjectList,
) apimachinerywait.ConditionWithContextFunc {
	return c.ResourcesMatch(list, c.IsManagedResourceReadyAndReady)
}
