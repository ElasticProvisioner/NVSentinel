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

package event

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/maintenance/armmaintenance"
	pb "github.com/nvidia/nvsentinel/data-models/pkg/protos"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAzureNormalizer_Normalize_PendingUpdate tests normalizing a pending maintenance update
func TestAzureNormalizer_Normalize_PendingUpdate(t *testing.T) {
	normalizer := &AzureNormalizer{}

	// 1. Define inputs
	pendingStatus := armmaintenance.UpdateStatusPending
	restartImpact := armmaintenance.ImpactTypeRestart
	notBefore := time.Date(2025, 12, 1, 10, 0, 0, 0, time.UTC)
	impactDuration := int32(300)

	update := &armmaintenance.Update{
		Status:              &pendingStatus,
		ImpactType:          &restartImpact,
		NotBefore:           &notBefore,
		ImpactDurationInSec: &impactDuration,
	}

	nodeInfo := map[string]interface{}{
		"nodeName":      "test-node",
		"providerID":    "azure:///subscriptions/sub-123/resourceGroups/rg-test/providers/Microsoft.Compute/virtualMachines/vm-test",
		"clusterName":   "test-cluster",
		"resourceGroup": "rg-test",
		"vmName":        "vm-test",
		"resourceID":    "/subscriptions/sub-123/resourcegroups/rg-test/providers/Microsoft.Compute/virtualMachines/vm-test",
	}

	// 2. Define expected output (excluding time-based fields that are set during normalization)
	expectedEvent := &model.MaintenanceEvent{
		CSP:                model.CSPAzure,
		ClusterName:        "test-cluster",
		ResourceType:       "VirtualMachine",
		ResourceID:         "/subscriptions/sub-123/resourcegroups/rg-test/providers/Microsoft.Compute/virtualMachines/vm-test",
		MaintenanceType:    model.TypeScheduled,
		Status:             model.StatusDetected,
		CSPStatus:          model.CSPStatusPending,
		ScheduledStartTime: &notBefore,
		ScheduledEndTime:   nil,
		RecommendedAction:  pb.RecommendedAction_RESTART_VM.String(),
		NodeName:           "test-node",
		Metadata: map[string]string{
			"resourceGroup":       "rg-test",
			"vmName":              "vm-test",
			"providerID":          "azure:///subscriptions/sub-123/resourceGroups/rg-test/providers/Microsoft.Compute/virtualMachines/vm-test",
			"resourceID":          "/subscriptions/sub-123/resourcegroups/rg-test/providers/Microsoft.Compute/virtualMachines/vm-test",
			"impactType":          "Restart",
			"impactDurationInSec": "300",
			"updateStatus":        "Pending",
		},
	}

	// 3. Call Normalize
	event, err := normalizer.Normalize(update, nodeInfo)
	require.NoError(t, err, "Normalize should not return an error")
	require.NotNil(t, event, "Event should not be nil")

	// 4. Deep comparison (excluding time-based fields)
	assert.Equal(t, expectedEvent.CSP, event.CSP)
	assert.Equal(t, expectedEvent.ClusterName, event.ClusterName)
	assert.Equal(t, expectedEvent.ResourceType, event.ResourceType)
	assert.Equal(t, expectedEvent.ResourceID, event.ResourceID)
	assert.Equal(t, expectedEvent.MaintenanceType, event.MaintenanceType)
	assert.Equal(t, expectedEvent.Status, event.Status)
	assert.Equal(t, expectedEvent.CSPStatus, event.CSPStatus)
	assert.Equal(t, expectedEvent.ScheduledStartTime, event.ScheduledStartTime)
	assert.Equal(t, expectedEvent.ScheduledEndTime, event.ScheduledEndTime)
	assert.Equal(t, expectedEvent.RecommendedAction, event.RecommendedAction)
	assert.Equal(t, expectedEvent.NodeName, event.NodeName)
	assert.Equal(t, expectedEvent.Metadata, event.Metadata)

	// Verify time-based fields are set and reasonable
	assert.NotEmpty(t, event.EventID, "EventID should be generated")
	assert.Contains(t, event.EventID, "azure-rg-test-vm-test", "EventID should contain expected format")
	assert.NotZero(t, event.EventReceivedTimestamp, "EventReceivedTimestamp should be set")
	assert.NotZero(t, event.LastUpdatedTimestamp, "LastUpdatedTimestamp should be set")
}

// TestAzureNormalizer_Normalize_InProgressUpdate tests normalizing an in-progress maintenance update
func TestAzureNormalizer_Normalize_InProgressUpdate(t *testing.T) {
	normalizer := &AzureNormalizer{}

	// 1. Define inputs
	inProgressStatus := armmaintenance.UpdateStatusInProgress
	redeployImpact := armmaintenance.ImpactTypeRedeploy

	update := &armmaintenance.Update{
		Status:     &inProgressStatus,
		ImpactType: &redeployImpact,
	}

	nodeInfo := map[string]interface{}{
		"nodeName":      "node-2",
		"providerID":    "azure:///subscriptions/sub-456/resourceGroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-prod",
		"clusterName":   "prod-cluster",
		"resourceGroup": "rg-prod",
		"vmName":        "vm-prod",
		"resourceID":    "/subscriptions/sub-456/resourcegroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-prod",
	}

	// 2. Define expected output (excluding time-based fields that are set during normalization)
	expectedEvent := &model.MaintenanceEvent{
		CSP:                model.CSPAzure,
		ClusterName:        "prod-cluster",
		ResourceType:       "VirtualMachine",
		ResourceID:         "/subscriptions/sub-456/resourcegroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-prod",
		MaintenanceType:    model.TypeScheduled,
		Status:             model.StatusMaintenanceOngoing,
		CSPStatus:          model.CSPStatusOngoing,
		ScheduledStartTime: nil,
		ScheduledEndTime:   nil,
		RecommendedAction:  pb.RecommendedAction_RESTART_VM.String(),
		NodeName:           "node-2",
		Metadata: map[string]string{
			"resourceGroup": "rg-prod",
			"vmName":        "vm-prod",
			"providerID":    "azure:///subscriptions/sub-456/resourceGroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-prod",
			"resourceID":    "/subscriptions/sub-456/resourcegroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-prod",
			"impactType":    "Redeploy",
			"updateStatus":  "InProgress",
		},
	}

	// 3. Call Normalize
	event, err := normalizer.Normalize(update, nodeInfo)
	require.NoError(t, err, "Normalize should not return an error")
	require.NotNil(t, event, "Event should not be nil")

	// 4. Deep comparison (excluding time-based fields)
	assert.Equal(t, expectedEvent.CSP, event.CSP)
	assert.Equal(t, expectedEvent.ClusterName, event.ClusterName)
	assert.Equal(t, expectedEvent.ResourceType, event.ResourceType)
	assert.Equal(t, expectedEvent.ResourceID, event.ResourceID)
	assert.Equal(t, expectedEvent.MaintenanceType, event.MaintenanceType)
	assert.Equal(t, expectedEvent.Status, event.Status)
	assert.Equal(t, expectedEvent.CSPStatus, event.CSPStatus)
	assert.Equal(t, expectedEvent.ScheduledStartTime, event.ScheduledStartTime)
	assert.Equal(t, expectedEvent.ScheduledEndTime, event.ScheduledEndTime)
	assert.Equal(t, expectedEvent.RecommendedAction, event.RecommendedAction)
	assert.Equal(t, expectedEvent.NodeName, event.NodeName)
	assert.Equal(t, expectedEvent.Metadata, event.Metadata)

	// Verify time-based fields are set and reasonable
	assert.NotEmpty(t, event.EventID, "EventID should be generated")
	assert.Contains(t, event.EventID, "azure-rg-prod-vm-prod", "EventID should contain expected format")
	assert.NotZero(t, event.EventReceivedTimestamp, "EventReceivedTimestamp should be set")
	assert.NotZero(t, event.LastUpdatedTimestamp, "LastUpdatedTimestamp should be set")
}
