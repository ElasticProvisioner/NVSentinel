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

package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/nvsentinel/internal/generated/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/deviceapiserver/cache"
)

func newTestService(t *testing.T) *GpuService {
	t.Helper()
	logger := klog.Background()
	c := cache.New(logger, nil)
	return NewGpuService(c)
}

func TestCreateGpu_Idempotent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	req := &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-0"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
			Status:   &v1alpha1.GpuStatus{},
		},
	}

	// First create should succeed
	gpu1, err := svc.CreateGpu(ctx, req)
	if err != nil {
		t.Fatalf("First CreateGpu failed: %v", err)
	}
	if gpu1 == nil {
		t.Fatal("First CreateGpu returned nil GPU")
	}

	// Second create (idempotent) should succeed and return non-nil GPU
	gpu2, err := svc.CreateGpu(ctx, req)
	if err != nil {
		t.Fatalf("Second CreateGpu failed: %v", err)
	}
	if gpu2 == nil {
		t.Fatal("Second CreateGpu returned nil GPU — error from Get() was swallowed")
	}
	if gpu2.GetMetadata().GetName() != "gpu-0" {
		t.Errorf("Expected name=gpu-0, got %s", gpu2.GetMetadata().GetName())
	}
}

func TestCreateGpu_Validation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  *v1alpha1.CreateGpuRequest
	}{
		{
			name: "nil gpu",
			req:  &v1alpha1.CreateGpuRequest{},
		},
		{
			name: "nil metadata",
			req: &v1alpha1.CreateGpuRequest{
				Gpu: &v1alpha1.Gpu{
					Spec: &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
				},
			},
		},
		{
			name: "empty name",
			req: &v1alpha1.CreateGpuRequest{
				Gpu: &v1alpha1.Gpu{
					Metadata: &v1alpha1.ObjectMeta{Name: ""},
					Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateGpu(ctx, tt.req)
			if err == nil {
				t.Error("Expected error for invalid request")
			}
		})
	}
}

func TestGetGpu(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create a GPU first
	createReq := &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-get"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-GET-1234"},
			Status:   &v1alpha1.GpuStatus{},
		},
	}
	_, err := svc.CreateGpu(ctx, createReq)
	if err != nil {
		t.Fatalf("CreateGpu failed: %v", err)
	}

	// Get existing GPU
	resp, err := svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-get"})
	if err != nil {
		t.Fatalf("GetGpu failed: %v", err)
	}
	if resp.GetGpu().GetMetadata().GetName() != "gpu-get" {
		t.Errorf("Expected name=gpu-get, got %s", resp.GetGpu().GetMetadata().GetName())
	}

	// Get non-existent GPU
	_, err = svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-nonexistent"})
	if err == nil {
		t.Error("Expected error for non-existent GPU")
	}

	// Get with empty name
	_, err = svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: ""})
	if err == nil {
		t.Error("Expected error for empty name")
	}
}

func TestListGpus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// List empty cache
	resp, err := svc.ListGpus(ctx, &v1alpha1.ListGpusRequest{})
	if err != nil {
		t.Fatalf("ListGpus failed: %v", err)
	}
	if len(resp.GetGpuList().GetItems()) != 0 {
		t.Errorf("Expected 0 GPUs, got %d", len(resp.GetGpuList().GetItems()))
	}

	// Create GPUs
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("gpu-%d", i)
		_, err := svc.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
			Gpu: &v1alpha1.Gpu{
				Metadata: &v1alpha1.ObjectMeta{Name: name},
				Spec:     &v1alpha1.GpuSpec{Uuid: fmt.Sprintf("GPU-%d", i)},
				Status:   &v1alpha1.GpuStatus{},
			},
		})
		if err != nil {
			t.Fatalf("CreateGpu failed: %v", err)
		}
	}

	// List should return 3
	resp, err = svc.ListGpus(ctx, &v1alpha1.ListGpusRequest{})
	if err != nil {
		t.Fatalf("ListGpus failed: %v", err)
	}
	if len(resp.GetGpuList().GetItems()) != 3 {
		t.Errorf("Expected 3 GPUs, got %d", len(resp.GetGpuList().GetItems()))
	}
}

func TestDeleteGpu(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create then delete
	_, err := svc.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-del"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-DEL"},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu failed: %v", err)
	}

	_, err = svc.DeleteGpu(ctx, &v1alpha1.DeleteGpuRequest{Name: "gpu-del"})
	if err != nil {
		t.Fatalf("DeleteGpu failed: %v", err)
	}

	// Verify deleted
	_, err = svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-del"})
	if err == nil {
		t.Error("Expected error for deleted GPU")
	}

	// Delete non-existent should fail
	_, err = svc.DeleteGpu(ctx, &v1alpha1.DeleteGpuRequest{Name: "gpu-nonexistent"})
	if err == nil {
		t.Error("Expected error for non-existent GPU")
	}
}

// createTestGpu is a helper to create a GPU and return it.
func createTestGpu(t *testing.T, svc *GpuService, name, uuid string) *v1alpha1.Gpu {
	t.Helper()
	gpu, err := svc.CreateGpu(context.Background(), &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: name},
			Spec:     &v1alpha1.GpuSpec{Uuid: uuid},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu(%s) failed: %v", name, err)
	}
	return gpu
}

func TestUpdateGpuStatus_Success(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create a GPU
	created := createTestGpu(t, svc, "gpu-status", "GPU-STATUS-1234")

	// Update its status
	newStatus := &v1alpha1.GpuStatus{
		Conditions: []*v1alpha1.Condition{
			{
				Type:               "NVMLReady",
				Status:             "True",
				Reason:             "Healthy",
				Message:            "GPU is healthy",
				LastTransitionTime: timestamppb.Now(),
			},
		},
	}

	updated, err := svc.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name:   "gpu-status",
		Status: newStatus,
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus failed: %v", err)
	}

	// Verify status was updated
	if len(updated.GetStatus().GetConditions()) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(updated.GetStatus().GetConditions()))
	}
	if updated.GetStatus().GetConditions()[0].GetType() != "NVMLReady" {
		t.Errorf("Expected condition type NVMLReady, got %s", updated.GetStatus().GetConditions()[0].GetType())
	}

	// Verify spec was preserved
	if updated.GetSpec().GetUuid() != "GPU-STATUS-1234" {
		t.Errorf("Spec was not preserved: expected UUID GPU-STATUS-1234, got %s", updated.GetSpec().GetUuid())
	}

	// Verify resource version advanced
	createdRV, _ := strconv.ParseInt(created.GetMetadata().GetResourceVersion(), 10, 64)
	updatedRV, _ := strconv.ParseInt(updated.GetMetadata().GetResourceVersion(), 10, 64)
	if updatedRV <= createdRV {
		t.Errorf("Resource version should have advanced: created=%d, updated=%d", createdRV, updatedRV)
	}
}

func TestUpdateGpuStatus_NotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name: "nonexistent",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True"},
			},
		},
	})
	if err == nil {
		t.Fatal("Expected error for non-existent GPU")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.NotFound {
		t.Errorf("Expected NotFound, got %v", err)
	}
}

func TestUpdateGpuStatus_Conflict(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestGpu(t, svc, "gpu-conflict", "GPU-CONFLICT")

	// Update with wrong resource version
	_, err := svc.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name:            "gpu-conflict",
		ResourceVersion: "999",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True"},
			},
		},
	})
	if err == nil {
		t.Fatal("Expected error for version conflict")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Aborted {
		t.Errorf("Expected Aborted, got %v", err)
	}
}

func TestUpdateGpuStatus_Validation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  *v1alpha1.UpdateGpuStatusRequest
	}{
		{
			name: "empty name",
			req:  &v1alpha1.UpdateGpuStatusRequest{Name: "", Status: &v1alpha1.GpuStatus{}},
		},
		{
			name: "nil status",
			req:  &v1alpha1.UpdateGpuStatusRequest{Name: "gpu-0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateGpuStatus(ctx, tt.req)
			if err == nil {
				t.Error("Expected error for invalid request")
			}
			if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
				t.Errorf("Expected InvalidArgument, got %v", err)
			}
		})
	}
}

func TestUpdateGpuStatus_PreservesSpec(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create GPU with spec
	createTestGpu(t, svc, "gpu-preserve", "GPU-PRESERVE-UUID")

	// Update status
	updated, err := svc.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name: "gpu-preserve",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "NVMLReady", Status: "False", Reason: "XIDError", Message: "Critical XID"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus failed: %v", err)
	}

	// Verify spec is unchanged
	if updated.GetSpec().GetUuid() != "GPU-PRESERVE-UUID" {
		t.Errorf("Spec UUID changed: expected GPU-PRESERVE-UUID, got %s", updated.GetSpec().GetUuid())
	}

	// Verify via Get
	resp, err := svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-preserve"})
	if err != nil {
		t.Fatalf("GetGpu failed: %v", err)
	}
	if resp.GetGpu().GetSpec().GetUuid() != "GPU-PRESERVE-UUID" {
		t.Errorf("Spec UUID changed after Get: expected GPU-PRESERVE-UUID, got %s", resp.GetGpu().GetSpec().GetUuid())
	}
	if resp.GetGpu().GetStatus().GetConditions()[0].GetType() != "NVMLReady" {
		t.Errorf("Status not updated via Get")
	}
}

func TestUpdateGpu_Conflict(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created := createTestGpu(t, svc, "gpu-upd-conflict", "GPU-UPD-CONFLICT")

	// Update with wrong version
	created.Metadata.ResourceVersion = "999"
	_, err := svc.UpdateGpu(ctx, &v1alpha1.UpdateGpuRequest{Gpu: created})
	if err == nil {
		t.Fatal("Expected error for version conflict")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Aborted {
		t.Errorf("Expected Aborted, got %v", err)
	}
}

func TestListGpus_ResourceVersionAtomic(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create some GPUs
	for i := 0; i < 5; i++ {
		createTestGpu(t, svc, fmt.Sprintf("gpu-rv-%d", i), fmt.Sprintf("GPU-RV-%d", i))
	}

	// List should return a resource version
	resp, err := svc.ListGpus(ctx, &v1alpha1.ListGpusRequest{})
	if err != nil {
		t.Fatalf("ListGpus failed: %v", err)
	}

	rv := resp.GetGpuList().GetMetadata().GetResourceVersion()
	if rv == "" {
		t.Error("Expected non-empty resource version")
	}

	rvInt, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		t.Fatalf("Failed to parse resource version: %v", err)
	}
	if rvInt < 5 {
		t.Errorf("Resource version should be at least 5 (one per GPU), got %d", rvInt)
	}
}

func TestFullCRUDCycle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create
	created := createTestGpu(t, svc, "gpu-crud", "GPU-CRUD-UUID")
	if created.GetMetadata().GetResourceVersion() == "" {
		t.Fatal("Created GPU has no resource version")
	}

	// Get
	resp, err := svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-crud"})
	if err != nil {
		t.Fatalf("GetGpu failed: %v", err)
	}
	if resp.GetGpu().GetSpec().GetUuid() != "GPU-CRUD-UUID" {
		t.Errorf("Expected UUID GPU-CRUD-UUID, got %s", resp.GetGpu().GetSpec().GetUuid())
	}

	// Update status
	updated, err := svc.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name: "gpu-crud",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "NVMLReady", Status: "True", Reason: "Healthy"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus failed: %v", err)
	}

	// Update full GPU
	updated.Status.Conditions[0].Status = "False"
	updated.Status.Conditions[0].Reason = "XIDError"
	_, err = svc.UpdateGpu(ctx, &v1alpha1.UpdateGpuRequest{Gpu: updated})
	if err != nil {
		t.Fatalf("UpdateGpu failed: %v", err)
	}

	// List
	listResp, err := svc.ListGpus(ctx, &v1alpha1.ListGpusRequest{})
	if err != nil {
		t.Fatalf("ListGpus failed: %v", err)
	}
	if len(listResp.GetGpuList().GetItems()) != 1 {
		t.Fatalf("Expected 1 GPU in list, got %d", len(listResp.GetGpuList().GetItems()))
	}

	// Delete
	_, err = svc.DeleteGpu(ctx, &v1alpha1.DeleteGpuRequest{Name: "gpu-crud"})
	if err != nil {
		t.Fatalf("DeleteGpu failed: %v", err)
	}

	// Verify deleted
	_, err = svc.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-crud"})
	if err == nil {
		t.Error("Expected error for deleted GPU")
	}
}
