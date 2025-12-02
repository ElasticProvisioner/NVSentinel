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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	v1informer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// Client encapsulates all state required to poll Azure for
// maintenance events and forward them to the main pipeline.
type Client struct {
	config         config.AzureConfig
	updatesClient  *armmaintenance.UpdatesClient
	normalizer     eventpkg.Normalizer
	clusterName    string
	kubeconfigPath string
	subscriptionID string
	nodeInformer   v1informer.NodeInformer
}

// NewClient builds and initialises a new Azure monitoring Client.
func NewClient(
	ctx context.Context,
	cfg config.AzureConfig,
	clusterName string,
	kubeconfigPath string,
) (*Client, error) {
	subscriptionID, err := getSubscriptionID(cfg)
	if err != nil {
		return nil, err
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	updatesClient, err := armmaintenance.NewUpdatesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure maintenance client: %w", err)
	}

	slog.Info("Successfully initialized Azure VM and maintenance clients", "subscriptionID", subscriptionID)

	var (
		k8sClient     kubernetes.Interface
		k8sRestConfig *rest.Config
	)

	slog.Info("Azure Client: Using kubeconfig from path", "path", kubeconfigPath)

	k8sRestConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
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

	nodeInformer, _ := newNodeInformer(k8sClient)

	normalizer, err := eventpkg.GetNormalizer(model.CSPAzure)
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure normalizer: %w", err)
	}

	return &Client{
		config:         cfg,
		updatesClient:  updatesClient,
		normalizer:     normalizer,
		clusterName:    clusterName,
		kubeconfigPath: kubeconfigPath,
		subscriptionID: subscriptionID,
		nodeInformer:   nodeInformer,
	}, nil
}

func newNodeInformer(k8sClient kubernetes.Interface) (v1informer.NodeInformer, chan struct{}) {
	factory := informers.NewSharedInformerFactory(k8sClient, 0*time.Second)

	nodeInformer := factory.Core().V1().Nodes()
	stopCh := make(chan struct{})

	_, err := nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		DeleteFunc: func(obj interface{}) {},
		UpdateFunc: func(oldObj, newObj interface{}) {},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to add event handler: %v", err)
		return nil, nil
	}

	factory.Start(stopCh)

	synced := factory.WaitForCacheSync(stopCh)
	for v, ok := range synced {
		if !ok {
			fmt.Fprintf(os.Stderr, "caches failed to sync: %v", v)
			return nil, nil
		}
	}

	return nodeInformer, stopCh
}

func (c *Client) GetName() model.CSP {
	return model.CSPAzure
}

func (c *Client) StartMonitoring(ctx context.Context, eventChan chan<- model.MaintenanceEvent) error {
	slog.Info("Starting Azure VM maintenance monitoring",
		"intervalSeconds", c.config.PollingIntervalSeconds)

	ticker := time.NewTicker(time.Duration(c.config.PollingIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			slog.Info("Azure monitoring stopping due to context cancellation.")
			return ctx.Err()
		}

		c.pollForMaintenanceEvents(ctx, eventChan)

		select {
		case <-ctx.Done():
			slog.Info("Azure monitoring stopping due to context cancellation.")
			return ctx.Err()
		case <-ticker.C:
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

	nodeList, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		metrics.CSPAPIErrors.WithLabelValues(string(model.CSPAzure), "list_nodes_error").Inc()
		slog.Error("Failed to list Kubernetes nodes", "error", err)

		return
	}

	slog.Debug("Found nodes to check for maintenance events", "count", len(nodeList))

	var wg sync.WaitGroup
	for _, node := range nodeList {
		wg.Add(1)

		go func(node *v1.Node) {
			defer wg.Done()

			c.processNodeMaintenanceEvents(ctx, node, eventChan)
		}(node)
	}

	wg.Wait()

	slog.Debug("Completed Azure maintenance event poll")
}

// processNodeMaintenanceEvents processes maintenance events for a single node
func (c *Client) processNodeMaintenanceEvents(
	ctx context.Context,
	node *v1.Node,
	eventChan chan<- model.MaintenanceEvent) {
	resourceGroup, vmName, err := parseAzureProviderID(node.Spec.ProviderID)
	if err != nil {
		slog.Warn("Failed to parse Azure provider ID",
			"node", node.Name,
			"providerID", node.Spec.ProviderID,
			"error", err)

		return
	}

	// Rebuild the resource ID needed for event normalization
	resourceID := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.Compute/virtualMachines/%s",
		c.subscriptionID, resourceGroup, vmName)

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

		for _, update := range page.Value {
			if c.shouldReportUpdate(update) {
				c.normalizeAndSendEvent(ctx, node, update, resourceGroup, vmName, resourceID, eventChan)
			}
		}
	}
}

// shouldReportUpdate determines if a maintenance update should be reported
func (c *Client) shouldReportUpdate(update *armmaintenance.Update) bool {
	// These are the two fields we need to determine if a maintenance event needs to be reported
	if update.Status == nil || update.ImpactType == nil {
		return false
	}

	// Only report updates that are not completed and have an actual impact
	return *update.Status != armmaintenance.UpdateStatusCompleted && *update.ImpactType != armmaintenance.ImpactTypeNone
}

// normalizeAndSendEvent normalizes a maintenance update and sends it to the event channel
func (c *Client) normalizeAndSendEvent(
	ctx context.Context,
	node *v1.Node,
	update *armmaintenance.Update,
	resourceGroup string,
	vmName string,
	resourceID string,
	eventChan chan<- model.MaintenanceEvent,
) {
	metrics.CSPEventsReceived.WithLabelValues(string(model.CSPAzure)).Inc()

	slog.Info("Detected Azure maintenance event",
		"node", node.Name,
		"resourceGroup", resourceGroup,
		"vmName", vmName,
		"status", *update.Status,
		"impactType", *update.ImpactType,
	)

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

		return
	}

	select {
	case eventChan <- *event:
		slog.Debug("Sent maintenance event to channel",
			"eventID", event.EventID,
			"node", event.NodeName)
	case <-ctx.Done():
		slog.Info("Context cancelled while sending event")
	}
}

func getSubscriptionID(cfg config.AzureConfig) (string, error) {
	if cfg.SubscriptionID != "" {
		return cfg.SubscriptionID, nil
	}

	// pulled from https://github.com/Microsoft/azureimds/blob/master/imdssample.go
	var PTransport = &http.Transport{Proxy: nil}

	client := http.Client{Transport: PTransport}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://169.254.169.254/metadata/instance", nil)
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
		return "", fmt.Errorf("failed to decode IMDS response: %w", err)
	}

	return result.Compute.SubscriptionID, nil
}

// parseAzureProviderID parses the provider ID to extract the resource group and VM name.
// Example provider ID:
// azure:///subscriptions/<sub-id>/resourceGroups/<resource-group>/providers/Microsoft.Compute/virtualMachines/<vm-name>
func parseAzureProviderID(providerID string) (string, string, error) {
	parts := strings.Split(providerID, "/")
	if len(parts) < 9 {
		return "", "", fmt.Errorf("invalid provider ID format: %s", providerID)
	}

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
