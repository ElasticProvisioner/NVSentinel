package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/maintenance/armmaintenance"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/config"
	eventpkg "github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/event"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/metrics"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client encapsulates all state required to poll Azure for
// maintenance events and forward them to the main pipeline.
type Client struct {
	config         config.AzureConfig
	updatesClient  *armmaintenance.UpdatesClient
	k8sClient      kubernetes.Interface
	normalizer     eventpkg.Normalizer
	clusterName    string
	kubeconfigPath string
	subscriptionID string
}

// NewClient builds and initialises a new Azure monitoring Client.
func NewClient(
	ctx context.Context,
	cfg config.AzureConfig,
	clusterName string,
	kubeconfigPath string,
) (*Client, error) {
	// Get the Azure subscription ID from config or IMDS
	subscriptionID, err := getSubscriptionID(cfg)
	if err != nil {
		return nil, err
	}

	// Create an Azure client
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create Azure credential: %w", err)
	}

	// Create maintenance updates client
	updatesClient, err := armmaintenance.NewUpdatesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create Azure maintenance client: %w", err)
	}

	slog.Info("Successfully initialized Azure VM and maintenance clients", "subscriptionID", subscriptionID)

	// Initialize Kubernetes client
	var k8sClient kubernetes.Interface
	var k8sRestConfig *rest.Config

	if kubeconfigPath != "" {
		slog.Info("Azure Client: Using kubeconfig from path", "path", kubeconfigPath)
		k8sRestConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		slog.Info("Azure Client: KubeconfigPath not specified, attempting in-cluster config")
		k8sRestConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		metrics.CSPMonitorErrors.WithLabelValues(string(model.CSPAzure), "k8s_config_error").Inc()
		return nil, fmt.Errorf("failed to load Kubernetes config: %w", err)
	}

	k8sClient, err = kubernetes.NewForConfig(k8sRestConfig)
	if err != nil {
		metrics.CSPMonitorErrors.WithLabelValues(string(model.CSPAzure), "k8s_client_error").Inc()
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	slog.Info("Azure Client: Successfully initialized Kubernetes client")

	// Create normalizer
	normalizer, err := eventpkg.GetNormalizer(model.CSPAzure)
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure normalizer: %w", err)
	}

	return &Client{
		config:         cfg,
		updatesClient:  updatesClient,
		k8sClient:      k8sClient,
		normalizer:     normalizer,
		clusterName:    clusterName,
		kubeconfigPath: kubeconfigPath,
		subscriptionID: subscriptionID,
	}, nil
}

func (c *Client) GetName() model.CSP {
	return model.CSPAzure
}

func (c *Client) StartMonitoring(ctx context.Context, eventChan chan<- model.MaintenanceEvent) error {
	slog.Info("Starting Azure VM maintenance monitoring",
		"intervalSeconds", c.config.PollingIntervalSeconds)

	// Perform initial poll immediately to ensure a query even if the context was cancelled
	if ctx.Err() == nil {
		c.pollForMaintenanceEvents(ctx, eventChan)
	} else {
		slog.Info("Azure monitoring not starting initial poll due to context cancellation.")
		return ctx.Err()
	}

	ticker := time.NewTicker(time.Duration(c.config.PollingIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Azure monitoring stopping due to context cancellation.")
			return ctx.Err()
		case <-ticker.C:
			c.pollForMaintenanceEvents(ctx, eventChan)
		}
	}
}

// pollForMaintenanceEvents checks all cluster nodes for Azure maintenance events in parallel.
func (c *Client) pollForMaintenanceEvents(ctx context.Context, eventChan chan<- model.MaintenanceEvent) {
	pollStart := time.Now()

	defer func() {
		metrics.CSPPollingDuration.WithLabelValues(string(model.CSPAzure)).Observe(time.Since(pollStart).Seconds())
	}()

	slog.Debug("Polling Azure for VM maintenance events")

	// List all nodes in the cluster
	nodeList, err := c.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		metrics.CSPAPIErrors.WithLabelValues(string(model.CSPAzure), "list_nodes_error").Inc()
		slog.Error("Failed to list Kubernetes nodes", "error", err)
		return
	}

	slog.Debug("Found nodes to check for maintenance events", "count", len(nodeList.Items))

	// Check each node for maintenance events in parallel
	var wg sync.WaitGroup
	for _, node := range nodeList.Items {
		wg.Add(1)
		go func(node v1.Node) {
			defer wg.Done()

			// Parse the Azure provider ID
			resourceGroup, vmName, err := parseAzureProviderID(node.Spec.ProviderID)
			if err != nil {
				slog.Warn("Failed to parse Azure provider ID",
					"node", node.Name,
					"providerID", node.Spec.ProviderID,
					"error", err)
				return
			}

			// Rebuild the resource ID needed in a couple places here
			resourceID := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.Compute/virtualMachines/%s",
				c.subscriptionID, resourceGroup, vmName)

			// Query the Azure Maintenance Updates API
			pager := c.updatesClient.NewListPager(
				resourceGroup,
				"Microsoft.Compute",
				"virtualMachines",
				vmName,
				nil,
			)

			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					metrics.CSPAPIErrors.WithLabelValues(string(model.CSPAzure), "updates_list_error").Inc()
					slog.Error("Failed to get Azure maintenance updates",
						"node", node.Name,
						"resourceGroup", resourceGroup,
						"vmName", vmName,
						"error", err)
					return
				}

				// Process each update
				for _, update := range page.Value {
					// These are the two fields we need to determine if a maintenance
					// event needs to be reported
					if update.Status == nil || update.ImpactType == nil {
						continue
					}

					if *update.Status != armmaintenance.UpdateStatusCompleted && *update.ImpactType != armmaintenance.ImpactTypeNone {
						metrics.CSPEventsReceived.WithLabelValues(string(model.CSPAzure)).Inc()

						slog.Info("Detected Azure maintenance event",
							"node", node.Name,
							"resourceGroup", resourceGroup,
							"vmName", vmName,
							"status", *update.Status,
							"impactType", *update.ImpactType,
						)

						// Normalize the event using the normalizer
						nodeInfo := map[string]interface{}{
							"nodeName":      node.Name,
							"providerID":    node.Spec.ProviderID,
							"clusterName":   c.clusterName,
							"resourceGroup": resourceGroup,
							"vmName":        vmName,
							"resourceID":    resourceID,
							"update":        update,
						}

						event, err := c.normalizer.Normalize(update, nodeInfo)
						if err != nil {
							slog.Error("Failed to normalize Azure maintenance event",
								"node", node.Name,
								"error", err)
							continue
						}

						// Send the event to the channel
						select {
						case eventChan <- *event:
							slog.Debug("Sent maintenance event to channel",
								"eventID", event.EventID,
								"node", event.NodeName)
						case <-ctx.Done():
							slog.Info("Context cancelled while sending event")
							return
						}
					}
				}
			}
		}(node)
	}

	// Wait for all node checks to complete
	wg.Wait()

	slog.Debug("Completed Azure maintenance event poll")
}

func getSubscriptionID(cfg config.AzureConfig) (string, error) {
	if cfg.SubscriptionID != "" {
		return cfg.SubscriptionID, nil
	}

	// pulled from https://github.com/Microsoft/azureimds/blob/master/imdssample.go
	var PTransport = &http.Transport{Proxy: nil}

	client := http.Client{Transport: PTransport}

	req, _ := http.NewRequest("GET", "http://169.254.169.254/metadata/instance", nil)
	req.Header.Add("Metadata", "True")

	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", "2021-02-01")
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// now that we have the response get the subscription ID from it
	var result struct {
		Compute struct {
			SubscriptionID string `json:"subscriptionId"`
		} `json:"compute"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode IMDS response: %v", err)
	}
	return result.Compute.SubscriptionID, nil
}

// parseAzureProviderID parses the provider ID to extract the resource group and VM name.
// Example provider ID: azure:///subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.Compute/virtualMachines/<vm-name>
func parseAzureProviderID(providerID string) (string, string, error) {
	parts := strings.Split(providerID, "/")
	if len(parts) < 9 {
		return "", "", fmt.Errorf("invalid provider ID format: %s", providerID)
	}

	// Extract resource group and VM name from the provider ID
	// Format: azure:///subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.Compute/virtualMachines/<vm-name>
	var resourceGroup, vmName string
	for i, part := range parts {
		if part == "resourceGroups" && i+1 < len(parts) {
			resourceGroup = parts[i+1]
		}
		if part == "virtualMachines" && i+1 < len(parts) {
			vmName = parts[i+1]
		}
	}

	if resourceGroup == "" || vmName == "" {
		return "", "", fmt.Errorf("could not extract resource group or VM name from provider ID: %s", providerID)
	}

	return resourceGroup, vmName, nil
}
