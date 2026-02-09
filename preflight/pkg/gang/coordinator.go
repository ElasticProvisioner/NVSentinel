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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// ConfigMapPrefix is the prefix for all preflight gang ConfigMaps.
	ConfigMapPrefix = "preflight-"

	// ConfigMapLabelGangID is the label for gang ID on ConfigMaps.
	ConfigMapLabelGangID = "nvsentinel.nvidia.com/gang-id"

	// ConfigMapLabelManagedBy is the label indicating this ConfigMap is managed by preflight.
	ConfigMapLabelManagedBy = "nvsentinel.nvidia.com/managed-by"

	// DataKeyExpectedCount is the ConfigMap data key for expected peer count.
	DataKeyExpectedCount = "expected_count"

	// DataKeyMasterAddr is the ConfigMap data key for the master (rank 0) IP address.
	DataKeyMasterAddr = "master_addr"

	// DataKeyMasterPort is the ConfigMap data key for the master port.
	DataKeyMasterPort = "master_port"

	// DataKeyPeers is the ConfigMap data key for the peer list.
	DataKeyPeers = "peers"

	// DefaultMasterPort is the default port for PyTorch distributed TCP bootstrap.
	DefaultMasterPort = 29500
)

// CoordinatorConfig contains configuration for the gang coordinator.
type CoordinatorConfig struct {
	// MasterPort is the port used for PyTorch distributed TCP bootstrap.
	// Default: 29500
	MasterPort int
}

// DefaultCoordinatorConfig returns the default coordinator configuration.
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		MasterPort: DefaultMasterPort,
	}
}

// Coordinator manages ConfigMaps for gang coordination.
// It creates and updates ConfigMaps that init containers read to discover peers.
type Coordinator struct {
	kubeClient kubernetes.Interface
	config     CoordinatorConfig
}

// NewCoordinator creates a new gang coordinator.
func NewCoordinator(kubeClient kubernetes.Interface, config CoordinatorConfig) *Coordinator {
	if config.MasterPort == 0 {
		config.MasterPort = DefaultMasterPort
	}

	return &Coordinator{
		kubeClient: kubeClient,
		config:     config,
	}
}

// MaxConfigMapNameLength is the maximum length of a Kubernetes resource name.
const MaxConfigMapNameLength = 63

// ConfigMapName returns the ConfigMap name for a given gang ID.
// The name is sanitized to be a valid DNS subdomain name and truncated
// with a hash suffix if it exceeds 63 characters.
func ConfigMapName(gangID string) string {
	// Sanitize gang ID to be a valid ConfigMap name
	name := strings.ToLower(gangID)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")

	fullName := ConfigMapPrefix + name

	// If within limits, return as-is
	if len(fullName) <= MaxConfigMapNameLength {
		return fullName
	}

	// Truncate and add hash suffix for uniqueness
	// Hash suffix is 8 chars, plus 1 for separator = 9 chars reserved
	hash := sha256.Sum256([]byte(gangID))
	hashSuffix := hex.EncodeToString(hash[:])[:8]

	// Truncate the name portion to fit: 63 - len("preflight-") - 1 (separator) - 8 (hash) = 44
	maxNameLen := MaxConfigMapNameLength - len(ConfigMapPrefix) - 1 - 8
	truncatedName := name[:maxNameLen]

	// Remove trailing dashes from truncation
	truncatedName = strings.TrimRight(truncatedName, "-")

	return ConfigMapPrefix + truncatedName + "-" + hashSuffix
}

// RegisterPeer registers a pod as a peer in the gang ConfigMap.
// Creates the ConfigMap if it doesn't exist.
func (c *Coordinator) RegisterPeer(ctx context.Context, namespace string, gangInfo *GangInfo, peer PeerInfo) error {
	if gangInfo == nil {
		return fmt.Errorf("gangInfo is required")
	}

	configMapName := ConfigMapName(gangInfo.GangID)

	slog.Debug("Registering peer in gang ConfigMap",
		"configMap", configMapName,
		"namespace", namespace,
		"gangID", gangInfo.GangID,
		"peer", peer.PodName,
		"peerIP", peer.PodIP)

	// Try to get existing ConfigMap
	cm, err := c.kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
		}

		// Create new ConfigMap
		cm = c.createConfigMap(configMapName, namespace, gangInfo)
	}

	// Update peer list
	c.addPeerToConfigMap(cm, peer)

	// Update master address (rank 0 is determined by alphabetical sort of pod names)
	c.updateMasterAddr(cm)

	// Create or update the ConfigMap
	if cm.ResourceVersion == "" {
		_, err = c.kubeClient.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			// Handle race condition where another controller created it
			if errors.IsAlreadyExists(err) {
				return c.RegisterPeer(ctx, namespace, gangInfo, peer)
			}

			return fmt.Errorf("failed to create ConfigMap %s: %w", configMapName, err)
		}

		slog.Info("Created gang ConfigMap",
			"configMap", configMapName,
			"namespace", namespace,
			"gangID", gangInfo.GangID)
	} else {
		_, err = c.kubeClient.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			// Handle conflict by retrying
			if errors.IsConflict(err) {
				return c.RegisterPeer(ctx, namespace, gangInfo, peer)
			}

			return fmt.Errorf("failed to update ConfigMap %s: %w", configMapName, err)
		}
	}

	slog.Info("Registered peer in gang ConfigMap",
		"configMap", configMapName,
		"namespace", namespace,
		"peer", peer.PodName,
		"peerIP", peer.PodIP)

	return nil
}

// GetGangConfigMap retrieves the gang ConfigMap.
func (c *Coordinator) GetGangConfigMap(ctx context.Context, namespace, gangID string) (*corev1.ConfigMap, error) {
	configMapName := ConfigMapName(gangID)

	cm, err := c.kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	return cm, nil
}

// ParsePeers parses the peers string from a ConfigMap into a slice of PeerInfo.
// Format: "podName:podIP" per line.
func ParsePeers(peersData string) []PeerInfo {
	var peers []PeerInfo

	lines := strings.Split(strings.TrimSpace(peersData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		peers = append(peers, PeerInfo{
			PodName: strings.TrimSpace(parts[0]),
			PodIP:   strings.TrimSpace(parts[1]),
		})
	}

	return peers
}

// GetRank returns the rank of a pod in the gang based on alphabetical ordering.
func GetRank(podName string, peers []PeerInfo) int {
	names := make([]string, len(peers))
	for i, p := range peers {
		names[i] = p.PodName
	}

	sort.Strings(names)

	for i, name := range names {
		if name == podName {
			return i
		}
	}

	return -1
}

// DeleteGangConfigMap deletes the gang ConfigMap.
func (c *Coordinator) DeleteGangConfigMap(ctx context.Context, namespace, gangID string) error {
	configMapName := ConfigMapName(gangID)

	err := c.kubeClient.CoreV1().ConfigMaps(namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap %s: %w", configMapName, err)
	}

	slog.Info("Deleted gang ConfigMap",
		"configMap", configMapName,
		"namespace", namespace,
		"gangID", gangID)

	return nil
}

// createConfigMap creates a new ConfigMap for gang coordination.
func (c *Coordinator) createConfigMap(name, namespace string, gangInfo *GangInfo) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ConfigMapLabelGangID:    gangInfo.GangID,
				ConfigMapLabelManagedBy: "preflight",
			},
		},
		Data: map[string]string{
			DataKeyExpectedCount: strconv.Itoa(gangInfo.ExpectedMinCount),
			DataKeyMasterPort:    strconv.Itoa(c.config.MasterPort),
			DataKeyPeers:         "",
			DataKeyMasterAddr:    "",
		},
	}
}

// addPeerToConfigMap adds a peer to the ConfigMap's peer list.
func (c *Coordinator) addPeerToConfigMap(cm *corev1.ConfigMap, peer PeerInfo) {
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	// Parse existing peers
	existingPeers := ParsePeers(cm.Data[DataKeyPeers])

	// Check if peer already exists (update IP if it changed)
	found := false

	for i, p := range existingPeers {
		if p.PodName == peer.PodName {
			existingPeers[i].PodIP = peer.PodIP
			found = true

			break
		}
	}

	if !found {
		existingPeers = append(existingPeers, peer)
	}

	// Sort peers by name for consistent ordering
	sort.Slice(existingPeers, func(i, j int) bool {
		return existingPeers[i].PodName < existingPeers[j].PodName
	})

	// Serialize peers back to string
	var lines []string
	for _, p := range existingPeers {
		if p.PodIP != "" {
			lines = append(lines, fmt.Sprintf("%s:%s", p.PodName, p.PodIP))
		}
	}

	cm.Data[DataKeyPeers] = strings.Join(lines, "\n")
}

// updateMasterAddr updates the master address in the ConfigMap.
// Master is the pod with rank 0 (first alphabetically).
func (c *Coordinator) updateMasterAddr(cm *corev1.ConfigMap) {
	peers := ParsePeers(cm.Data[DataKeyPeers])
	if len(peers) == 0 {
		return
	}

	// Sort by name to determine rank 0
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].PodName < peers[j].PodName
	})

	// Rank 0 is the master
	if peers[0].PodIP != "" {
		cm.Data[DataKeyMasterAddr] = peers[0].PodIP
	}
}
