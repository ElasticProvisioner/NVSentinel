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

// Package discoverer provides gang discovery implementations for different schedulers.
package discoverer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nvidia/nvsentinel/preflight/pkg/gang/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// PodGroupConfig defines the configuration for a PodGroup-based gang discoverer.
type PodGroupConfig struct {
	// Name is the discoverer name (e.g., "volcano").
	Name string

	// AnnotationKeys are the annotation keys to check for pod group name (in order).
	AnnotationKeys []string

	// LabelKeys are optional label keys to check as fallback (in order).
	LabelKeys []string

	// PodGroupGVR is the GroupVersionResource for the PodGroup CRD.
	PodGroupGVR schema.GroupVersionResource
}

// VolcanoConfig returns the configuration for Volcano Scheduler.
func VolcanoConfig() PodGroupConfig {
	return PodGroupConfig{
		Name: "volcano",
		AnnotationKeys: []string{
			"volcano.sh/pod-group",         // Standard PodGroup annotation
			"scheduling.k8s.io/group-name", // Volcano Job annotation
		},
		LabelKeys: []string{
			"volcano.sh/job-name", // Volcano Job pods label
		},
		PodGroupGVR: schema.GroupVersionResource{
			Group:    "scheduling.volcano.sh",
			Version:  "v1beta1",
			Resource: "podgroups",
		},
	}
}

// Presets maps scheduler names to their configurations.
var Presets = map[string]func() PodGroupConfig{
	"volcano": VolcanoConfig,
}

// PodGroupDiscoverer discovers gang members using PodGroup CRDs.
// This is a generic implementation that works with Volcano and similar PodGroup-based schedulers.
type PodGroupDiscoverer struct {
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
	config        PodGroupConfig
}

// NewPodGroupDiscoverer creates a new PodGroup-based gang discoverer.
func NewPodGroupDiscoverer(
	kubeClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	config PodGroupConfig,
) *PodGroupDiscoverer {
	return &PodGroupDiscoverer{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		config:        config,
	}
}

// Name returns the discoverer name.
func (d *PodGroupDiscoverer) Name() string {
	return d.config.Name
}

// CanHandle returns true if the pod belongs to a gang managed by this discoverer.
func (d *PodGroupDiscoverer) CanHandle(pod *corev1.Pod) bool {
	return d.getPodGroupName(pod) != ""
}

// ExtractGangID extracts the gang identifier from a pod.
func (d *PodGroupDiscoverer) ExtractGangID(pod *corev1.Pod) string {
	podGroupName := d.getPodGroupName(pod)
	if podGroupName == "" {
		return ""
	}

	return fmt.Sprintf("%s-%s-%s", d.config.Name, pod.Namespace, podGroupName)
}

// getPodGroupName extracts the pod group name from annotations or labels.
func (d *PodGroupDiscoverer) getPodGroupName(pod *corev1.Pod) string {
	// Check annotations first
	if pod.Annotations != nil {
		for _, key := range d.config.AnnotationKeys {
			if name := pod.Annotations[key]; name != "" {
				return name
			}
		}
	}

	// Check labels as fallback
	if pod.Labels != nil {
		for _, key := range d.config.LabelKeys {
			if name := pod.Labels[key]; name != "" {
				return name
			}
		}
	}

	return ""
}

// DiscoverPeers finds all pods in the same PodGroup.
func (d *PodGroupDiscoverer) DiscoverPeers(ctx context.Context, pod *corev1.Pod) (*types.GangInfo, error) {
	podGroupName := d.getPodGroupName(pod)
	if podGroupName == "" {
		slog.Debug("Pod not handled by discoverer",
			"discoverer", d.config.Name,
			"pod", pod.Name,
			"namespace", pod.Namespace)

		return nil, nil
	}

	gangID := d.ExtractGangID(pod)

	slog.Info("Discovering gang",
		"discoverer", d.config.Name,
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"podGroup", podGroupName,
		"gangID", gangID)

	// Get expected size from PodGroup CRD - required for correct gang coordination
	expectedCount, err := d.getPodGroupMinMember(ctx, pod.Namespace, podGroupName)
	if err != nil {
		return nil, fmt.Errorf("failed to get PodGroup %s/%s minMember (check RBAC): %w",
			pod.Namespace, podGroupName, err)
	}

	// List all pods in the namespace
	pods, err := d.kubeClient.CoreV1().Pods(pod.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", pod.Namespace, err)
	}

	var peers []types.PeerInfo

	for i := range pods.Items {
		p := &pods.Items[i]

		// Check if this pod belongs to the same gang
		if d.getPodGroupName(p) != podGroupName {
			continue
		}

		// Skip pods that are not running or pending
		if p.Status.Phase != corev1.PodRunning && p.Status.Phase != corev1.PodPending {
			continue
		}

		peers = append(peers, types.PeerInfo{
			PodName:   p.Name,
			PodIP:     p.Status.PodIP,
			NodeName:  p.Spec.NodeName,
			Namespace: p.Namespace,
		})
	}

	if len(peers) == 0 {
		slog.Warn("No peers found for gang",
			"discoverer", d.config.Name,
			"pod", pod.Name,
			"podGroup", podGroupName,
			"gangID", gangID)

		return nil, nil
	}

	slog.Info("Discovered gang",
		"discoverer", d.config.Name,
		"gangID", gangID,
		"podGroup", podGroupName,
		"expectedCount", expectedCount,
		"discoveredPeers", len(peers))

	return &types.GangInfo{
		GangID:           gangID,
		ExpectedMinCount: expectedCount,
		Peers:            peers,
	}, nil
}

// getPodGroupMinMember retrieves the minMember field from a PodGroup CRD.
func (d *PodGroupDiscoverer) getPodGroupMinMember(
	ctx context.Context,
	namespace, name string,
) (int, error) {
	if d.dynamicClient == nil {
		return 0, fmt.Errorf("dynamic client not configured")
	}

	gvr := d.config.PodGroupGVR

	podGroup, err := d.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get PodGroup %s/%s: %w", namespace, name, err)
	}

	minMember, found, err := unstructured.NestedInt64(podGroup.Object, "spec", "minMember")
	if err != nil {
		return 0, fmt.Errorf("failed to extract minMember from PodGroup: %w", err)
	}

	if !found {
		return 0, fmt.Errorf("PodGroup %s/%s has no spec.minMember field", namespace, name)
	}

	return int(minMember), nil
}
