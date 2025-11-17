package azure

import (
	"context"
	"fmt"

	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/model"
)

// Client encapsulates all state required to poll Azure for
// maintenance events and forward them to the main pipeline.
type Client struct{}

// NewClient builds and initialises a new Azure monitoring Client.
func NewClient() (*Client, error) {
	return nil, fmt.Errorf("NewClient is not implemented for Azure")
}

func (c *Client) GetName() model.CSP {
	return model.CSPAzure
}

func (c *Client) StartMonitoring(ctx context.Context, eventChan chan<- model.MaintenanceEvent) error {
	return fmt.Errorf("StartMonitoring is not implemented for Azure")
}
