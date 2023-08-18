package xpconditions

import (
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
