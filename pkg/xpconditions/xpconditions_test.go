package xpconditions

import (
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/e2e-framework/klient/k8s"
)

func TestConditions_awaitCondition(t *testing.T) {
	type args struct {
		obj             unstructured.Unstructured
		conditionType   string
		conditionStatus corev1.ConditionStatus
	}
	conditionSyncedTrue := map[string]interface{}{
		"type":   "Synced",
		"status": "True",
	}
	conditionReadyTrue := map[string]interface{}{
		"type":   "Ready",
		"status": "True",
	}
	conditionReadyFalse := map[string]interface{}{
		"type":   "Ready",
		"status": "False",
	}

	const TypeReady = "Ready"
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "existing condition, matches expectation",
			args: args{
				obj: unstructured.Unstructured{
					Object: map[string]interface{}{
						"status": map[string]interface{}{
							"conditions": []interface{}{
								conditionReadyTrue},
						},
					},
				},
				conditionType:   TypeReady,
				conditionStatus: corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "existing condition, doesnt match expectation",
			args: args{
				obj: unstructured.Unstructured{
					Object: map[string]interface{}{
						"Status": map[string]interface{}{
							"Conditions": []interface{}{
								conditionSyncedTrue,
								conditionReadyFalse,
							},
						},
					},
				},
				conditionType:   TypeReady,
				conditionStatus: corev1.ConditionFalse,
			},
			want: false,
		},
		{
			name: "non existing condition, doesnt match expectation",
			args: args{
				obj:             unstructured.Unstructured{},
				conditionType:   TypeReady,
				conditionStatus: corev1.ConditionFalse,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkCondition(&tt.args.obj, tt.args.conditionType, tt.args.conditionStatus); got != tt.want {
				t.Errorf("checkCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConditions_IsManagedResourceReadyAndReady(t *testing.T) {
	type args struct {
		object k8s.Object
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "No conditions, return false",
			args: args{
				object: &v1.DaemonSet{
					Status: v1.DaemonSetStatus{Conditions: []v1.DaemonSetCondition{}},
				},
			},
			want: false,
		},
		{
			name: "No matching condition, return false",
			args: args{
				object: &v1.DaemonSet{
					Status: v1.DaemonSetStatus{Conditions: []v1.DaemonSetCondition{
						{Type: "Foo", Status: corev1.ConditionTrue},
					}},
				},
			},
			want: false,
		},
		{
			name: "Synced=True condition, return false",
			args: args{
				object: &v1.DaemonSet{
					Status: v1.DaemonSetStatus{Conditions: []v1.DaemonSetCondition{
						{Type: "Synced", Status: corev1.ConditionTrue},
					}},
				},
			},
			want: false,
		},
		{
			name: "Synced=True,Ready=True conditions, return true",
			args: args{
				object: &v1.DaemonSet{
					Status: v1.DaemonSetStatus{Conditions: []v1.DaemonSetCondition{
						{Type: "Synced", Status: corev1.ConditionTrue},
						{Type: "Ready", Status: corev1.ConditionTrue},
					}},
				},
			},
			want: true,
		},
		{
			name: "Synced=Unknown,Ready=True conditions, return false",
			args: args{
				object: &v1.DaemonSet{
					Status: v1.DaemonSetStatus{Conditions: []v1.DaemonSetCondition{
						{Type: "Synced", Status: corev1.ConditionUnknown},
						{Type: "Ready", Status: corev1.ConditionTrue},
					}},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Conditions{}
			if got := c.IsManagedResourceReadyAndReady(tt.args.object); got != tt.want {
				t.Errorf("IsManagedResourceReadyAndReady() = %v, want %v", got, tt.want)
			}
		})
	}
}
