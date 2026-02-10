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

// Package gang provides gang scheduling discovery and coordination for multi-node workloads.
package gang

import (
	"fmt"

	"github.com/nvidia/nvsentinel/preflight/pkg/config"
	"github.com/nvidia/nvsentinel/preflight/pkg/gang/coordinator"
	"github.com/nvidia/nvsentinel/preflight/pkg/gang/discoverer"
	"github.com/nvidia/nvsentinel/preflight/pkg/gang/types"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Re-export types for convenience.
type (
	PeerInfo          = types.PeerInfo
	GangInfo          = types.GangInfo
	GangDiscoverer    = types.GangDiscoverer
	Coordinator       = coordinator.Coordinator
	CoordinatorConfig = coordinator.CoordinatorConfig
)

// Re-export coordinator functions.
var (
	ConfigMapName            = coordinator.ConfigMapName
	NewCoordinator           = coordinator.NewCoordinator
	DefaultCoordinatorConfig = coordinator.DefaultCoordinatorConfig
	ParsePeers               = coordinator.ParsePeers
	GetRank                  = coordinator.GetRank
)

// NewDiscovererFromConfig creates a gang discoverer from configuration.
// Supports both built-in presets (scheduler field) and custom configs.
func NewDiscovererFromConfig(
	cfg config.GangDiscoveryConfig,
	kubeClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
) (GangDiscoverer, error) {
	// Handle custom config
	if cfg.Custom != nil {
		return newCustomDiscoverer(cfg.Custom, kubeClient, dynamicClient)
	}

	// Handle built-in presets
	if cfg.Scheduler == "" {
		return nil, fmt.Errorf("gangDiscovery.scheduler or gangDiscovery.custom is required")
	}

	// Kubernetes native (workloadRef) is special - not PodGroup-based
	if cfg.Scheduler == "kubernetes" {
		return discoverer.NewWorkloadRefDiscoverer(kubeClient), nil
	}

	// PodGroup-based presets
	presetFn, ok := discoverer.Presets[cfg.Scheduler]
	if !ok {
		validPresets := []string{"kubernetes"}
		for name := range discoverer.Presets {
			validPresets = append(validPresets, name)
		}

		return nil, fmt.Errorf("unknown scheduler: %q (valid: %v, or use custom)", cfg.Scheduler, validPresets)
	}

	return discoverer.NewPodGroupDiscoverer(kubeClient, dynamicClient, presetFn()), nil
}

// newCustomDiscoverer creates a PodGroup discoverer from custom config.
func newCustomDiscoverer(
	cfg *config.CustomSchedulerConfig,
	kubeClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
) (GangDiscoverer, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("gangDiscovery.custom.name is required")
	}

	if len(cfg.AnnotationKeys) == 0 && len(cfg.LabelKeys) == 0 {
		return nil, fmt.Errorf("gangDiscovery.custom must have at least one annotationKey or labelKey")
	}

	gvr := cfg.PodGroupGVR
	if gvr.Group == "" || gvr.Version == "" || gvr.Resource == "" {
		return nil, fmt.Errorf("gangDiscovery.custom.podGroupGVR requires group, version, and resource")
	}

	podGroupConfig := discoverer.PodGroupConfig{
		Name:           cfg.Name,
		AnnotationKeys: cfg.AnnotationKeys,
		LabelKeys:      cfg.LabelKeys,
		PodGroupGVR: schema.GroupVersionResource{
			Group:    cfg.PodGroupGVR.Group,
			Version:  cfg.PodGroupGVR.Version,
			Resource: cfg.PodGroupGVR.Resource,
		},
	}

	return discoverer.NewPodGroupDiscoverer(kubeClient, dynamicClient, podGroupConfig), nil
}
