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

package azure

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/maintenance/armmaintenance"
	fakearm "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/maintenance/armmaintenance/fake"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/config"
	eventpkg "github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/event"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

// TestPollForMaintenanceEvents_NoMaintenanceEvents tests the basic happy path
// where we have one node but it returns no maintenance updates
func TestPollForMaintenanceEvents_NoMaintenanceEvents(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Spec: v1.NodeSpec{
			ProviderID: "azure:///subscriptions/test-sub-id/resourceGroups/test-rg/providers/Microsoft.Compute/virtualMachines/test-vm",
		},
	}
	fakeK8sClient := fakek8s.NewSimpleClientset(node)

	// Create a fake Azure Updates server that returns no updates
	fakeUpdatesServer := fakearm.UpdatesServer{
		NewListPager: func(resourceGroupName, providerName, resourceType, resourceName string, options *armmaintenance.UpdatesClientListOptions) (resp fake.PagerResponder[armmaintenance.UpdatesClientListResponse]) {
			// Return an empty list of updates (no maintenance events)
			resp.AddPage(http.StatusOK, armmaintenance.UpdatesClientListResponse{
				ListUpdatesResult: armmaintenance.ListUpdatesResult{
					Value: []*armmaintenance.Update{},
				},
			}, nil)
			return
		},
	}

	updatesClientOptions := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fakearm.NewUpdatesServerTransport(&fakeUpdatesServer),
		},
	}
	updatesClient, err := armmaintenance.NewUpdatesClient("test-sub-id", &fake.TokenCredential{}, updatesClientOptions)
	require.NoError(t, err, "Failed to create updates client")

	client := &Client{
		config: config.AzureConfig{
			PollingIntervalSeconds: 60,
		},
		updatesClient:  updatesClient,
		k8sClient:      fakeK8sClient,
		normalizer:     &eventpkg.AzureNormalizer{},
		clusterName:    "test-cluster",
		subscriptionID: "test-sub-id",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventChan := make(chan model.MaintenanceEvent, 10)

	client.pollForMaintenanceEvents(ctx, eventChan)

	// Verify no events were sent (since we have no maintenance updates)
	select {
	case event := <-eventChan:
		assert.Fail(t, "Expected no events", "Received unexpected event: %+v", event)
	default:
		// Expected: no events in channel
	}
}

// TestPollForMaintenanceEvents_OneMaintenanceEvent tests that a pending maintenance
// event is detected and sent to the event channel
func TestPollForMaintenanceEvents_OneMaintenanceEvent(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Spec: v1.NodeSpec{
			ProviderID: "azure:///subscriptions/test-sub-id/resourceGroups/test-rg/providers/Microsoft.Compute/virtualMachines/test-vm",
		},
	}
	fakeK8sClient := fakek8s.NewSimpleClientset(node)

	// Create a pending maintenance update that should trigger an event
	pendingStatus := armmaintenance.UpdateStatusPending
	restartImpact := armmaintenance.ImpactTypeRestart
	notBefore := time.Now().Add(1 * time.Hour)

	maintenanceUpdate := &armmaintenance.Update{
		Status:     &pendingStatus,
		ImpactType: &restartImpact,
		NotBefore:  &notBefore,
	}

	// Create a fake Azure Updates server that returns one pending update
	fakeUpdatesServer := fakearm.UpdatesServer{
		NewListPager: func(resourceGroupName, providerName, resourceType, resourceName string, options *armmaintenance.UpdatesClientListOptions) (resp fake.PagerResponder[armmaintenance.UpdatesClientListResponse]) {
			// Return one pending maintenance update
			resp.AddPage(http.StatusOK, armmaintenance.UpdatesClientListResponse{
				ListUpdatesResult: armmaintenance.ListUpdatesResult{
					Value: []*armmaintenance.Update{maintenanceUpdate},
				},
			}, nil)
			return
		},
	}

	updatesClientOptions := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fakearm.NewUpdatesServerTransport(&fakeUpdatesServer),
		},
	}
	updatesClient, err := armmaintenance.NewUpdatesClient("test-sub-id", &fake.TokenCredential{}, updatesClientOptions)
	require.NoError(t, err, "Failed to create updates client")

	client := &Client{
		config: config.AzureConfig{
			PollingIntervalSeconds: 60,
		},
		updatesClient:  updatesClient,
		k8sClient:      fakeK8sClient,
		normalizer:     &eventpkg.AzureNormalizer{},
		clusterName:    "test-cluster",
		subscriptionID: "test-sub-id",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventChan := make(chan model.MaintenanceEvent, 10)

	client.pollForMaintenanceEvents(ctx, eventChan)

	// Verify exactly one event was sent
	select {
	case event := <-eventChan:
		// Verify basic properties of the event
		assert.Equal(t, model.CSPAzure, event.CSP, "Event should be from Azure CSP")
		assert.Equal(t, "test-cluster", event.ClusterName, "Event should have correct cluster name")
		assert.Equal(t, "test-node-1", event.NodeName, "Event should have correct node name")
		assert.Equal(t, model.StatusDetected, event.Status, "Event status should be DETECTED")
		assert.Equal(t, model.TypeScheduled, event.MaintenanceType, "Event should be SCHEDULED maintenance type")
		assert.NotEmpty(t, event.EventID, "Event should have a non-empty EventID")
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Expected to receive an event, but timeout occurred")
	}

	// Verify no additional events were sent
	select {
	case event := <-eventChan:
		assert.Fail(t, "Expected only one event", "Received additional event: %+v", event)
	default:
		// Expected: no more events in channel
	}
}
