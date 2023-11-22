package xpconditions

import (
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	v1 "github.com/crossplane/crossplane/apis/pkg/v1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/e2e-framework/klient/k8s"
)

func Test_convertToManaged(t *testing.T) {
	type args struct {
		object k8s.Object
	}
	tests := []struct {
		name string
		args args
		want resource.Managed
	}{
		{
			name: "happy path",
			args: args{
				object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "foo.crossplane.io/v1",
						"kind":       "Foo",
						"metadata": map[string]interface{}{
							"name": "example-resource",
						},
						// Should be ignored
						"spec": map[string]interface{}{
							"forProvider": map[string]interface{}{
								"propertyKey": "value",
							},
						},
						"status": map[string]interface{}{
							"atProvider": map[string]interface{}{
								"providerState": true,
							},
							"conditions": []interface{}{
								map[string]interface{}{
									"reason": "Available",
									"status": "True",
									"type":   "Ready",
								},
								map[string]interface{}{
									"reason": "ReconcileSuccess",
									"status": "True",
									"type":   "Synced",
								},
							},
						},
					},
				},
			},
			want: &DummyManaged{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Foo",
					APIVersion: "foo.crossplane.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "example-resource",
				},
				ConditionedStatus: xpv1.ConditionedStatus{
					Conditions: []xpv1.Condition{
						{
							Type:   xpv1.TypeReady,
							Status: corev1.ConditionTrue,
							Reason: xpv1.ReasonAvailable,
						},
						{
							Type:   xpv1.TypeSynced,
							Status: corev1.ConditionTrue,
							Reason: xpv1.ReasonReconcileSuccess,
						},
					},
				},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToManaged(tt.args.object)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("convertToManaged() = %v, want %v, diff: %v", got, tt.want, diff)
			}
		})
	}
}

func Test_managedCheckCondition(t *testing.T) {
	type args struct {
		o             resource.Managed
		conditionType xpv1.ConditionType
		want          corev1.ConditionStatus
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "resolves an existing condition Ready",
			args: args{
				o: &DummyManaged{
					ObjectMeta: metav1.ObjectMeta{
						Name: "example-resource",
					},
					ConditionedStatus: xpv1.ConditionedStatus{
						Conditions: []xpv1.Condition{
							{
								Type:   xpv1.TypeReady,
								Status: corev1.ConditionTrue,
								Reason: xpv1.ReasonAvailable,
							},
						},
					},
				},
				conditionType: xpv1.TypeReady,
				want:          corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "resolves an existing condition Synced",
			args: args{
				o: &DummyManaged{
					ObjectMeta: metav1.ObjectMeta{
						Name: "example-resource",
					},
					ConditionedStatus: xpv1.ConditionedStatus{
						Conditions: []xpv1.Condition{
							{
								Type:   xpv1.TypeReady,
								Status: corev1.ConditionTrue,
								Reason: xpv1.ReasonAvailable,
							},
						},
					},
				},
				conditionType: xpv1.TypeSynced,
				want:          corev1.ConditionFalse,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := managedCheckCondition(tt.args.o, tt.args.conditionType, tt.args.want); got != tt.want {
				t.Errorf("managedCheckCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConditions_awaitCondition(t *testing.T) {
	type args struct {
		provider        v1.Provider
		conditionType   xpv1.ConditionType
		conditionStatus corev1.ConditionStatus
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "existing condition, matches expectation",
			args: args{
				provider: v1.Provider{Status: v1.ProviderStatus{
					ConditionedStatus: xpv1.ConditionedStatus{
						Conditions: []xpv1.Condition{
							{Type: xpv1.TypeReady, Status: corev1.ConditionTrue},
						},
					},
				}},
				conditionType:   xpv1.TypeReady,
				conditionStatus: corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "existing condition, doesnt match expectation",
			args: args{
				provider: v1.Provider{Status: v1.ProviderStatus{
					ConditionedStatus: xpv1.ConditionedStatus{
						Conditions: []xpv1.Condition{
							{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue},
							{Type: xpv1.TypeReady, Status: corev1.ConditionTrue},
						},
					},
				}},
				conditionType:   xpv1.TypeReady,
				conditionStatus: corev1.ConditionFalse,
			},
			want: false,
		},
		{
			name: "non existing condition, doesnt match expectation",
			args: args{
				provider:        v1.Provider{Status: v1.ProviderStatus{}},
				conditionType:   xpv1.TypeReady,
				conditionStatus: corev1.ConditionFalse,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&tt.args.provider)
			if err != nil {
				t.Error(err)
			}
			unstr := unstructured.Unstructured{Object: data}
			if got := awaitCondition(&unstr, tt.args.conditionType, tt.args.conditionStatus); got != tt.want {
				t.Errorf("awaitCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}
