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
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/maintenance/armmaintenance"
	pb "github.com/nvidia/nvsentinel/data-models/pkg/protos"
	"github.com/nvidia/nvsentinel/health-monitors/csp-health-monitor/pkg/model"
)

type AzureNormalizer struct{}

func (n *AzureNormalizer) Normalize(rawEvent interface{}, additionalInfo ...interface{}) (*model.MaintenanceEvent, error) {
	// We need the normalizer interface but expect a struct
	// from the azure sdk
	update, ok := rawEvent.(*armmaintenance.Update)
	if !ok {
		return nil, fmt.Errorf("expected *armmaintenance.Update, got %T", rawEvent)
	}

	if len(additionalInfo) == 0 {
		return nil, fmt.Errorf("missing node information in additionalInfo")
	}

	nodeInfo, ok := additionalInfo[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{} for nodeInfo, got %T", additionalInfo[0])
	}

	nodeName, _ := nodeInfo["nodeName"].(string)
	providerID, _ := nodeInfo["providerID"].(string)
	clusterName, _ := nodeInfo["clusterName"].(string)
	resourceGroup, _ := nodeInfo["resourceGroup"].(string)
	vmName, _ := nodeInfo["vmName"].(string)
	resourceID, _ := nodeInfo["resourceID"].(string)

	now := time.Now().UTC()

	eventID := fmt.Sprintf("azure-%s-%s-%d", resourceGroup, vmName, now.Unix())

	metadata := map[string]string{
		"resourceGroup": resourceGroup,
		"vmName":        vmName,
		"providerID":    providerID,
		"resourceID":    resourceID,
	}

	if update.MaintenanceScope != nil {
		metadata["maintenanceScope"] = string(*update.MaintenanceScope)
	}
	if update.ImpactType != nil {
		metadata["impactType"] = string(*update.ImpactType)
	}
	if update.ImpactDurationInSec != nil {
		metadata["impactDurationInSec"] = fmt.Sprintf("%d", *update.ImpactDurationInSec)
	}
	if update.Status != nil {
		metadata["updateStatus"] = string(*update.Status)
	}

	status := mapAzureStatusToInternal(update.Status)
	cspStatus := mapAzureStatusToCSPStatus(update.Status)

	event := &model.MaintenanceEvent{
		EventID:                eventID,
		CSP:                    model.CSPAzure,
		ClusterName:            clusterName,
		ResourceType:           "VirtualMachine",
		ResourceID:             resourceID,
		MaintenanceType:        model.TypeScheduled,
		Status:                 status,
		CSPStatus:              cspStatus,
		ScheduledStartTime:     update.NotBefore,
		ScheduledEndTime:       nil, // Azure doesn't provide end time in Updates API
		EventReceivedTimestamp: now,
		LastUpdatedTimestamp:   now,
		RecommendedAction:      pb.RecommendedAction_RESTART_VM.String(),
		Metadata:               metadata,
		NodeName:               nodeName,
	}

	return event, nil
}

// mapAzureStatusToInternal maps status from the Azure SDK to the internal status
func mapAzureStatusToInternal(status *armmaintenance.UpdateStatus) model.InternalStatus {
	if status == nil {
		return model.StatusDetected
	}

	switch *status {
	case armmaintenance.UpdateStatusPending:
		return model.StatusDetected
	case armmaintenance.UpdateStatusInProgress:
		return model.StatusMaintenanceOngoing
	case armmaintenance.UpdateStatusCompleted:
		return model.StatusMaintenanceComplete
	case armmaintenance.UpdateStatusRetryNow, armmaintenance.UpdateStatusRetryLater:
		return model.StatusError
	default:
		return model.StatusDetected
	}
}

// mapAzureStatusToCSPStatus maps status from the Azure SDK to the CSP provider status
func mapAzureStatusToCSPStatus(status *armmaintenance.UpdateStatus) model.ProviderStatus {
	if status == nil {
		return model.CSPStatusUnknown
	}

	switch *status {
	case armmaintenance.UpdateStatusPending:
		return model.CSPStatusPending
	case armmaintenance.UpdateStatusInProgress:
		return model.CSPStatusOngoing
	case armmaintenance.UpdateStatusCompleted:
		return model.CSPStatusCompleted
	default:
		return model.CSPStatusUnknown
	}
}
