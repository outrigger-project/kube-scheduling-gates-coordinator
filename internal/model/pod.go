package model

import (
	"context"
	"fmt"
	ksgcv1beta1 "github.com/outrigger-project/kube-scheduling-gates-coordinator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"strconv"
)

type Pod struct {
	corev1.Pod
	ctx      context.Context
	recorder record.EventRecorder
}

func (pod *Pod) HasSchedulingGate(schedulingGateName string) bool {
	if pod.Spec.SchedulingGates == nil {
		// If the schedulingGates array is nil, we return false
		return false
	}
	for _, schedulingGate := range pod.Spec.SchedulingGates {
		if schedulingGate.Name == schedulingGateName {
			return true
		}
	}
	// the scheduling gate is not found.
	return false
}

// EnsureLabel ensures that the pod has the given label with the given value.
func (pod *Pod) EnsureLabel(label string, value string) {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[label] = value
}

// EnsureAndIncrementLabel ensures that the pod has the given label with the given value.
// If the label is already set, it increments the value.
func (pod *Pod) EnsureAndIncrementLabel(label string) {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	if _, ok := pod.Labels[label]; !ok {
		pod.Labels[label] = "1"
		return
	}
	cur, err := strconv.ParseInt(pod.Labels[label], 10, 32)
	if err != nil {
		pod.Labels[label] = "1"
	} else {
		pod.Labels[label] = fmt.Sprintf("%d", cur+1)
	}
}

func (pod *Pod) EnsureSortedSchedulingGates(sgo *ksgcv1beta1.SchedulingGatesOrdering) {
	if len(pod.Pod.Spec.SchedulingGates) == 0 {
		return
	}

	// Create a map for quick lookup of pod scheduling gates
	podSchedulingGateMap := make(map[string]corev1.PodSchedulingGate)
	for _, podSchedulingGate := range pod.Spec.SchedulingGates {
		podSchedulingGateMap[podSchedulingGate.Name] = podSchedulingGate
	}

	// Build the final sorted list based on the order in sgo
	final := make([]corev1.PodSchedulingGate, 0, len(sgo.Spec.SchedulingGates))
	for _, schedulingGate := range sgo.Spec.SchedulingGates {
		if podSchedulingGate, exists := podSchedulingGateMap[schedulingGate.Name]; exists {
			final = append(final, podSchedulingGate)
		}
	}

	pod.Spec.SchedulingGates = final
}

func (pod *Pod) OutOfOrderSchedulingGatesRemoval(oldPod *corev1.Pod) bool {
	if len(pod.Pod.Spec.SchedulingGates) >= len(oldPod.Spec.SchedulingGates) {
		// Note that == should be considered enough as this method is called during UPDATE operations, when only removal
		// of scheduling gates is allowed. If the number of scheduling gates is the same, it means that no scheduling gates
		// are being removed.
		return false
	}
	// if n scheduling gates are removed from the head of the list,
	// oldPod.SchedulingGates = oldPod.SchedulingGates[:n] + newPod.SchedulingGates[n:]
	n := len(oldPod.Spec.SchedulingGates) - len(pod.Spec.SchedulingGates) // n > 0 by design

	// Compare the remaining scheduling gates in order.
	for i := n; i < len(oldPod.Spec.SchedulingGates); i++ {
		if oldPod.Spec.SchedulingGates[i].Name != pod.Spec.SchedulingGates[i-n].Name {
			// A scheduling gate was removed from the middle of the list.
			return true
		}
	}

	return false
}
