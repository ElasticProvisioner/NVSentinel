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
}

// NewGangController creates a new gang controller.
func NewGangController(
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
	}

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onPodAdd,
		UpdateFunc: c.onPodUpdate,
	})

	return c
}

// RegisterPod is called by the webhook when a pod is admitted that belongs to a gang.
// This is a no-op since the informer handles everything, but kept for interface compatibility.
func (c *GangController) RegisterPod(_ webhook.GangRegistration) {
	// The informer's AddFunc/UpdateFunc handles registration when pod gets IP.
	// This callback exists so the webhook can notify us, but we don't need to track it.
}

func (c *GangController) Run(ctx context.Context) error {
	slog.Info("Starting gang controller")

	if !cache.WaitForCacheSync(ctx.Done(), c.podSynced) {
		return ctx.Err()
	}

	slog.Info("Gang controller cache synced")

	<-ctx.Done()

	return nil
}

func (c *GangController) onPodAdd(obj interface{}) {
	pod := obj.(*corev1.Pod)
	c.handlePod(context.Background(), pod)
}

func (c *GangController) onPodUpdate(oldObj, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	// Only process if IP changed
	if oldPod.Status.PodIP == newPod.Status.PodIP {
		return
	}

	c.handlePod(context.Background(), newPod)
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

	// Get gang info
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
