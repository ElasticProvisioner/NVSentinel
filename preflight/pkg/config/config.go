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

package config

import (
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

type Config struct {
	Port    int
	CertDir string

	FileConfig
}

type FileConfig struct {
	InitContainers       []corev1.Container     `yaml:"initContainers"`
	GPUResourceNames     []string               `yaml:"gpuResourceNames"`
	NetworkResourceNames []string               `yaml:"networkResourceNames"`
	DCGM                 DCGMConfig             `yaml:"dcgm"`
	GangDiscovery        GangDiscoveryConfig    `yaml:"gangDiscovery"`
	GangCoordination     GangCoordinationConfig `yaml:"gangCoordination"`
}

type DCGMConfig struct {
	HostengineAddr     string `yaml:"hostengineAddr"`
	DiagLevel          int    `yaml:"diagLevel"`
	ConnectorSocket    string `yaml:"connectorSocket"`
	ProcessingStrategy string `yaml:"processingStrategy"`
}

// GangDiscoveryConfig contains configuration for gang discovery.
type GangDiscoveryConfig struct {
	// Scheduler specifies which gang scheduler to use for discovery.
	// Options: workloadRef (K8s 1.35+), volcano, kueue, labels
	// Required when gangCoordination.enabled is true.
	Scheduler string `yaml:"scheduler"`

	// Labels contains configuration for label-based discovery.
	// Only used when scheduler is "labels".
	Labels LabelDiscoveryConfig `yaml:"labels,omitempty"`
}

// LabelDiscoveryConfig contains configuration for label-based gang discovery.
type LabelDiscoveryConfig struct {
	// GangIDLabel is the label key that identifies the gang.
	// All pods with the same value for this label are considered part of the same gang.
	// Default: "app.kubernetes.io/gang-id"
	GangIDLabel string `yaml:"gangIdLabel,omitempty"`

	// GangSizeLabel is the label key that specifies the expected gang size.
	// If not present on pods, size is determined by counting discovered pods.
	// Default: "app.kubernetes.io/gang-size"
	GangSizeLabel string `yaml:"gangSizeLabel,omitempty"`
}

// GangCoordinationConfig contains configuration for gang coordination.
type GangCoordinationConfig struct {
	// Enabled enables gang coordination for multi-node checks.
	Enabled bool `yaml:"enabled"`

	// Timeout is the maximum time to wait for all gang members to register.
	// Accepts duration strings like "10m", "5m30s", etc.
	// Default: 10m
	Timeout string `yaml:"timeout,omitempty"`

	// TimeoutDuration is the parsed Timeout value. Set by Load().
	TimeoutDuration time.Duration `yaml:"-"`

	// MasterPort is the port used for PyTorch distributed TCP bootstrap.
	// Default: 29500
	MasterPort int `yaml:"masterPort,omitempty"`

	// ConfigMapMountPath is the path where gang ConfigMap is mounted in init containers.
	// Default: /etc/preflight
	ConfigMapMountPath string `yaml:"configMapMountPath,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var fileConfig FileConfig
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if len(fileConfig.GPUResourceNames) == 0 {
		fileConfig.GPUResourceNames = []string{"nvidia.com/gpu"}
	}

	if fileConfig.DCGM.DiagLevel == 0 {
		fileConfig.DCGM.DiagLevel = 1
	}

	if fileConfig.DCGM.ProcessingStrategy == "" {
		fileConfig.DCGM.ProcessingStrategy = "EXECUTE_REMEDIATION"
	}

	// Set gang discovery defaults
	if fileConfig.GangDiscovery.Labels.GangIDLabel == "" {
		fileConfig.GangDiscovery.Labels.GangIDLabel = "app.kubernetes.io/gang-id"
	}

	if fileConfig.GangDiscovery.Labels.GangSizeLabel == "" {
		fileConfig.GangDiscovery.Labels.GangSizeLabel = "app.kubernetes.io/gang-size"
	}

	// Set gang coordination defaults
	if fileConfig.GangCoordination.Timeout == "" {
		fileConfig.GangCoordination.Timeout = "10m"
	}

	// Parse timeout string into time.Duration.
	// We use a string in the YAML config for user-friendliness ("10m" vs nanoseconds),
	// since Go's time.Duration doesn't natively unmarshal from JSON/YAML strings.
	timeout, err := time.ParseDuration(fileConfig.GangCoordination.Timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid gangCoordination.timeout %q: %w", fileConfig.GangCoordination.Timeout, err)
	}
	fileConfig.GangCoordination.TimeoutDuration = timeout

	if fileConfig.GangCoordination.MasterPort == 0 {
		fileConfig.GangCoordination.MasterPort = 29500
	}

	if fileConfig.GangCoordination.ConfigMapMountPath == "" {
		fileConfig.GangCoordination.ConfigMapMountPath = "/etc/preflight"
	}

	return &Config{FileConfig: fileConfig}, nil
}
