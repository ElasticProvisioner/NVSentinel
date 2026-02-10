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
	"testing"

	"github.com/nvidia/nvsentinel/preflight/pkg/config"
)

func TestNewDiscovererFromConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.GangDiscoveryConfig
		wantName  string
		wantError bool
	}{
		{
			name:     "kubernetes preset",
			cfg:      config.GangDiscoveryConfig{Scheduler: "kubernetes"},
			wantName: "kubernetes",
		},
		{
			name:     "volcano preset",
			cfg:      config.GangDiscoveryConfig{Scheduler: "volcano"},
			wantName: "volcano",
		},
		{
			name:      "unknown scheduler",
			cfg:       config.GangDiscoveryConfig{Scheduler: "unknown"},
			wantError: true,
		},
		{
			name: "custom config",
			cfg: config.GangDiscoveryConfig{
				Custom: &config.CustomSchedulerConfig{
					Name:           "my-scheduler",
					AnnotationKeys: []string{"my.io/pod-group"},
					PodGroupGVR: config.GVRConfig{
						Group:    "my.io",
						Version:  "v1",
						Resource: "podgroups",
					},
				},
			},
			wantName: "my-scheduler",
		},
		{
			name: "custom config missing name",
			cfg: config.GangDiscoveryConfig{
				Custom: &config.CustomSchedulerConfig{
					AnnotationKeys: []string{"my.io/pod-group"},
					PodGroupGVR: config.GVRConfig{
						Group: "my.io", Version: "v1", Resource: "podgroups",
					},
				},
			},
			wantError: true,
		},
		{
			name: "custom config missing keys",
			cfg: config.GangDiscoveryConfig{
				Custom: &config.CustomSchedulerConfig{
					Name: "my-scheduler",
					PodGroupGVR: config.GVRConfig{
						Group: "my.io", Version: "v1", Resource: "podgroups",
					},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDiscovererFromConfig(tt.cfg, nil, nil)

			if tt.wantError {
				if err == nil {
					t.Error("NewDiscovererFromConfig() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("NewDiscovererFromConfig() error = %v", err)
			}

			if got.Name() != tt.wantName {
				t.Errorf("Discoverer.Name() = %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}
