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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

// TestPollForMaintenanceEvents_NoMaintenanceEvents tests the basic happy path
// where we have one node but it returns no maintenance updates
func TestPollForMaintenanceEvents_NoMaintenanceEvents(t *testing.T) {
	// Create a fake Kubernetes client with a single node
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

	// Create the Azure Updates client with the fake server
	updatesClientOptions := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fakearm.NewUpdatesServerTransport(&fakeUpdatesServer),
		},
	}
	updatesClient, err := armmaintenance.NewUpdatesClient("test-sub-id", &fake.TokenCredential{}, updatesClientOptions)
	if err != nil {
		t.Fatalf("Failed to create updates client: %v", err)
	}

	// Create the client
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

	// Create a context with timeout to ensure the test doesn't hang
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create an event channel
	eventChan := make(chan model.MaintenanceEvent, 10)

	// Call the function under test
	client.pollForMaintenanceEvents(ctx, eventChan)

	// Verify no events were sent (since we have no maintenance updates)
	select {
	case event := <-eventChan:
		t.Errorf("Expected no events, but got: %+v", event)
	default:
		// Expected: no events in channel
		t.Log("Test passed: no events were sent when no maintenance updates exist")
	}
}
