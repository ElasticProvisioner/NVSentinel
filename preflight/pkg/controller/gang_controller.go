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

// Package controller provides controllers for managing preflight resources.
package controller

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nvidia/nvsentinel/preflight/pkg/gang"
	"github.com/nvidia/nvsentinel/preflight/pkg/webhook"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// GangController watches pods and updates gang ConfigMaps with peer information.
type GangController struct {
	kubeClient  kubernetes.Interface
	podSynced   cache.InformerSynced
	coordinator *gang.Coordinator
	discoverer  gang.GangDiscoverer
	ctx         context.Context
}

// NewGangController creates a new gang controller.
// ctx must be provided upfront to avoid nil-context race when informers deliver events.
func NewGangController(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	informerFactory informers.SharedInformerFactory,
	coordinator *gang.Coordinator,
	discoverer gang.GangDiscoverer,
) *GangController {
	podInformer := informerFactory.Core().V1().Pods()

	c := &GangController{
		kubeClient:  kubeClient,
		podSynced:   podInformer.Informer().HasSynced,
		coordinator: coordinator,
		discoverer:  discoverer,
		ctx:         ctx,
	}

	_, _ = podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onPodAdd,
		UpdateFunc: c.onPodUpdate,
	})

	return c
}

// RegisterPod is called by the webhook when a pod is admitted that belongs to a gang.
// It creates the ConfigMap immediately so schedulers (like KAI) that validate
// ConfigMap existence before scheduling won't block.
func (c *GangController) RegisterPod(ctx context.Context, reg webhook.GangRegistration) {
	if reg.GangID == "" {
		return
	}

	// Create ConfigMap immediately (with empty peer list).
	// This ensures it exists before the scheduler tries to validate it.
	// Peer IPs will be added later when pods get scheduled and receive IPs.
	if err := c.coordinator.EnsureConfigMap(ctx, reg.Namespace, reg.GangID, 0); err != nil {
		slog.Error("Failed to ensure gang ConfigMap",
			"namespace", reg.Namespace,
			"gangID", reg.GangID,
			"configMap", reg.ConfigMapName,
			"error", err)
	}
}

// Run starts the gang controller and waits for cache sync.
func (c *GangController) Run() error {
	slog.Info("Starting gang controller")

	if !cache.WaitForCacheSync(c.ctx.Done(), c.podSynced) {
		return c.ctx.Err()
	}

	slog.Info("Gang controller cache synced")

	<-c.ctx.Done()

	return nil
}

func (c *GangController) onPodAdd(obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		slog.Error("Unexpected object type in pod informer", "type", fmt.Sprintf("%T", obj))
		return
	}

	c.handlePod(c.ctx, pod)
}

func (c *GangController) onPodUpdate(oldObj, newObj any) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		slog.Error("Unexpected object type in pod informer", "type", fmt.Sprintf("%T", oldObj))
		return
	}

	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		slog.Error("Unexpected object type in pod informer", "type", fmt.Sprintf("%T", newObj))
		return
	}

	// Only process if IP changed
	if oldPod.Status.PodIP == newPod.Status.PodIP {
		return
	}

	c.handlePod(c.ctx, newPod)
}

func (c *GangController) handlePod(ctx context.Context, pod *corev1.Pod) {
	// Skip if pod doesn't have an IP yet
	if pod.Status.PodIP == "" {
		return
	}

	// Skip if pod is terminating
	if pod.DeletionTimestamp != nil {
		return
	}

	// Check if this pod belongs to a gang
	if c.discoverer == nil || !c.discoverer.CanHandle(pod) {
		return
	}

	gangID := c.discoverer.ExtractGangID(pod)
	if gangID == "" {
		return
	}

	gangInfo, err := c.discoverer.DiscoverPeers(ctx, pod)
	if err != nil {
		slog.Error("Failed to discover gang peers",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"gangID", gangID,
			"error", err)

		return
	}

	if gangInfo == nil {
		return
	}

	peer := gang.PeerInfo{
		PodName:   pod.Name,
		PodIP:     pod.Status.PodIP,
		NodeName:  pod.Spec.NodeName,
		Namespace: pod.Namespace,
	}

	if err := c.coordinator.RegisterPeer(ctx, pod.Namespace, gangInfo, peer); err != nil {
		slog.Error("Failed to register peer",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"gangID", gangID,
			"error", err)

		return
	}

	slog.Info("Registered gang peer",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"gangID", gangID,
		"podIP", pod.Status.PodIP)
}
