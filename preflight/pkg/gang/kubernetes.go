// Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gang

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// WorkloadGVR is the GroupVersionResource for K8s 1.35+ Workload API.
// See: https://kubernetes.io/blog/2025/12/29/kubernetes-v1-35-introducing-workload-aware-scheduling/
var WorkloadGVR = schema.GroupVersionResource{
	Group:    "scheduling.k8s.io",
	Version:  "v1alpha1",
	Resource: "workloads",
}

// WorkloadRefDiscoverer discovers gang members using K8s 1.35+ native workloadRef.
// Pods are linked to Workloads via spec.workloadRef:
//
//	spec:
//	  workloadRef:
//	    name: training-job-workload
//	    podGroup: workers
type WorkloadRefDiscoverer struct {
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
}

// NewWorkloadRefDiscoverer creates a new workloadRef gang discoverer.
func NewWorkloadRefDiscoverer(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface) *WorkloadRefDiscoverer {
	return &WorkloadRefDiscoverer{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
	}
}

func (w *WorkloadRefDiscoverer) Name() string {
	return "kubernetes"
}

// CanHandle returns true if the pod has a workloadRef.
func (w *WorkloadRefDiscoverer) CanHandle(pod *corev1.Pod) bool {
	// Check if pod has workloadRef in spec
	// Note: As of K8s 1.35, workloadRef is a new field in PodSpec
	// We check via unstructured since it may not be in client-go types yet
	return getWorkloadRefName(pod) != ""
}

// ExtractGangID extracts the gang identifier from a pod's workloadRef.
func (w *WorkloadRefDiscoverer) ExtractGangID(pod *corev1.Pod) string {
	workloadName := getWorkloadRefName(pod)
	podGroup := getWorkloadRefPodGroup(pod)

	if workloadName == "" {
		return ""
	}

	if podGroup != "" {
		return fmt.Sprintf("kubernetes-%s-%s-%s", pod.Namespace, workloadName, podGroup)
	}

	return fmt.Sprintf("kubernetes-%s-%s", pod.Namespace, workloadName)
}

// DiscoverPeers finds all pods with the same workloadRef.
func (w *WorkloadRefDiscoverer) DiscoverPeers(ctx context.Context, pod *corev1.Pod) (*GangInfo, error) {
	if !w.CanHandle(pod) {
		return nil, nil
	}

	workloadName := getWorkloadRefName(pod)
	podGroup := getWorkloadRefPodGroup(pod)
	gangID := w.ExtractGangID(pod)

	slog.Debug("Discovering workloadRef gang",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"workload", workloadName,
		"podGroup", podGroup,
		"gangID", gangID)

	// Get expected minCount from Workload CRD
	expectedMinCount, err := w.getWorkloadMinCount(ctx, pod.Namespace, workloadName, podGroup)
	if err != nil {
		slog.Warn("Failed to get Workload minCount, will use discovered pod count",
			"workload", workloadName,
			"error", err)
	}

	// List all pods in the namespace and filter by workloadRef
	pods, err := w.kubeClient.CoreV1().Pods(pod.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", pod.Namespace, err)
	}

	var peers []PeerInfo

	for i := range pods.Items {
		p := &pods.Items[i]

		// Check if pod has same workloadRef
		pWorkloadName := getWorkloadRefName(p)
		pPodGroup := getWorkloadRefPodGroup(p)

		if pWorkloadName != workloadName {
			continue
		}

		// If we're filtering by podGroup, check it matches
		if podGroup != "" && pPodGroup != podGroup {
			continue
		}

		// Skip pods that are not running or pending
		if p.Status.Phase != corev1.PodRunning && p.Status.Phase != corev1.PodPending {
			continue
		}

		peers = append(peers, PeerInfo{
			PodName:   p.Name,
			PodIP:     p.Status.PodIP,
			NodeName:  p.Spec.NodeName,
			Namespace: p.Namespace,
		})
	}

	if len(peers) == 0 {
		return nil, nil
	}

	// Use discovered count if Workload lookup failed
	if expectedMinCount == 0 {
		expectedMinCount = len(peers)
	}

	slog.Info("Discovered workloadRef gang",
		"gangID", gangID,
		"workload", workloadName,
		"podGroup", podGroup,
		"expectedMinCount", expectedMinCount,
		"discoveredPeers", len(peers))

	return &GangInfo{
		GangID:           gangID,
		ExpectedMinCount: expectedMinCount,
		Peers:            peers,
	}, nil
}

// getWorkloadMinCount retrieves the minCount from a Workload's podGroup gang policy.
func (w *WorkloadRefDiscoverer) getWorkloadMinCount(ctx context.Context, namespace, name, podGroup string) (int, error) {
	if w.dynamicClient == nil {
		return 0, fmt.Errorf("dynamic client not configured")
	}

	workload, err := w.dynamicClient.Resource(WorkloadGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get Workload %s/%s: %w", namespace, name, err)
	}

	// Navigate: spec.podGroups[].policy.gang.minCount
	podGroups, found, err := unstructured.NestedSlice(workload.Object, "spec", "podGroups")
	if err != nil || !found {
		return 0, fmt.Errorf("failed to get podGroups from Workload: %w", err)
	}

	for _, pg := range podGroups {
		pgMap, ok := pg.(map[string]any)
		if !ok {
			continue
		}

		pgName, _, _ := unstructured.NestedString(pgMap, "name")

		// If podGroup specified, match it; otherwise take first one
		if podGroup != "" && pgName != podGroup {
			continue
		}

		minCount, found, err := unstructured.NestedInt64(pgMap, "policy", "gang", "minCount")
		if err != nil || !found {
			continue
		}

		return int(minCount), nil
	}

	return 0, nil
}

// getWorkloadRefName extracts workloadRef.name from a pod.
// Returns empty string if not present.
func getWorkloadRefName(pod *corev1.Pod) string {
	// workloadRef is a new field in K8s 1.35+
	// Access via annotations as fallback for older client-go versions
	if pod.Annotations != nil {
		if name := pod.Annotations["scheduling.k8s.io/workload-name"]; name != "" {
			return name
		}
	}

	// TODO: Once client-go is updated for K8s 1.35+, use:
	// if pod.Spec.WorkloadRef != nil {
	//     return pod.Spec.WorkloadRef.Name
	// }

	return ""
}

// getWorkloadRefPodGroup extracts workloadRef.podGroup from a pod.
// Returns empty string if not present.
func getWorkloadRefPodGroup(pod *corev1.Pod) string {
	if pod.Annotations != nil {
		if pg := pod.Annotations["scheduling.k8s.io/workload-pod-group"]; pg != "" {
			return pg
		}
	}

	// TODO: Once client-go is updated for K8s 1.35+, use:
	// if pod.Spec.WorkloadRef != nil {
	//     return pod.Spec.WorkloadRef.PodGroup
	// }

	return ""
}
