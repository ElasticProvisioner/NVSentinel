// Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/device-api/api/gen/go/device/v1alpha1"
	"github.com/nvidia/device-api/pkg/deviceapiserver/cache"
)

func TestProviderService_RegisterGpu(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewProviderService(gpuCache)
	ctx := context.Background()

	// Test: Register with empty name
	t.Run("InvalidArgument_EmptyName", func(t *testing.T) {
		resp, err := svc.RegisterGpu(ctx, &RegisterGpuRequest{Name: ""})
		if resp != nil {
			t.Error("Expected nil response")
		}
		if err == nil {
			t.Fatal("Expected error")
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Test: Register new GPU
	t.Run("Success_NewGpu", func(t *testing.T) {
		resp, err := svc.RegisterGpu(ctx, &RegisterGpuRequest{
			Name:       "gpu-0",
			ProviderId: "test-provider",
			Spec:       &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		if !resp.Created {
			t.Error("Expected Created=true for new GPU")
		}
		if resp.ResourceVersion != 1 {
			t.Errorf("Expected ResourceVersion=1, got %d", resp.ResourceVersion)
		}
	})

	// Test: Register existing GPU (same name)
	t.Run("Success_ExistingGpu", func(t *testing.T) {
		resp, err := svc.RegisterGpu(ctx, &RegisterGpuRequest{
			Name:       "gpu-0",
			ProviderId: "test-provider",
			Spec:       &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		if resp.Created {
			t.Error("Expected Created=false for existing GPU")
		}
		// Resource version should stay the same
		if resp.ResourceVersion != 1 {
			t.Errorf("Expected ResourceVersion=1 (unchanged), got %d", resp.ResourceVersion)
		}
	})
}

func TestProviderService_UnregisterGpu(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewProviderService(gpuCache)
	ctx := context.Background()

	// Test: Unregister with empty name
	t.Run("InvalidArgument_EmptyName", func(t *testing.T) {
		resp, err := svc.UnregisterGpu(ctx, &UnregisterGpuRequest{Name: ""})
		if resp != nil {
			t.Error("Expected nil response")
		}
		if err == nil {
			t.Fatal("Expected error")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Test: Unregister non-existent GPU
	t.Run("NotExists", func(t *testing.T) {
		resp, err := svc.UnregisterGpu(ctx, &UnregisterGpuRequest{Name: "gpu-missing"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		if resp.Deleted {
			t.Error("Expected Deleted=false for non-existent GPU")
		}
	})

	// Register a GPU first
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-0"}, nil, "test")

	// Test: Unregister existing GPU
	t.Run("Success", func(t *testing.T) {
		resp, err := svc.UnregisterGpu(ctx, &UnregisterGpuRequest{Name: "gpu-0"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		if !resp.Deleted {
			t.Error("Expected Deleted=true")
		}

		// Verify GPU is gone
		_, found := gpuCache.Get("gpu-0")
		if found {
			t.Error("GPU should not exist after unregister")
		}
	})
}

func TestProviderService_UpdateGpuStatus(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewProviderService(gpuCache)
	ctx := context.Background()

	// Test: Update with empty name
	t.Run("InvalidArgument_EmptyName", func(t *testing.T) {
		resp, err := svc.UpdateGpuStatus(ctx, &UpdateGpuStatusRequest{Name: ""})
		if resp != nil {
			t.Error("Expected nil response")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Test: Update non-existent GPU
	t.Run("NotFound", func(t *testing.T) {
		resp, err := svc.UpdateGpuStatus(ctx, &UpdateGpuStatusRequest{
			Name:   "gpu-missing",
			Status: &v1alpha1.GpuStatus{},
		})
		if resp != nil {
			t.Error("Expected nil response")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", st.Code())
		}
	})

	// Register a GPU first
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-0"}, nil, "test")

	// Test: Update existing GPU status
	t.Run("Success", func(t *testing.T) {
		newStatus := &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True", Message: "GPU is ready"},
			},
		}
		resp, err := svc.UpdateGpuStatus(ctx, &UpdateGpuStatusRequest{
			Name:       "gpu-0",
			ProviderId: "test-provider",
			Status:     newStatus,
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		if resp.ResourceVersion != 2 {
			t.Errorf("Expected ResourceVersion=2, got %d", resp.ResourceVersion)
		}

		// Verify status was updated
		gpu, _ := gpuCache.Get("gpu-0")
		if len(gpu.Status.Conditions) != 1 {
			t.Errorf("Expected 1 condition, got %d", len(gpu.Status.Conditions))
		}
		if gpu.Status.Conditions[0].Status != "True" {
			t.Errorf("Expected status=True, got %s", gpu.Status.Conditions[0].Status)
		}
	})
}

func TestProviderService_UpdateGpuCondition(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewProviderService(gpuCache)
	ctx := context.Background()

	// Test: Update with empty name
	t.Run("InvalidArgument_EmptyName", func(t *testing.T) {
		resp, err := svc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:      "",
			Condition: &v1alpha1.Condition{Type: "Ready"},
		})
		if resp != nil {
			t.Error("Expected nil response")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Test: Update with nil condition
	t.Run("InvalidArgument_NilCondition", func(t *testing.T) {
		resp, err := svc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:      "gpu-0",
			Condition: nil,
		})
		if resp != nil {
			t.Error("Expected nil response")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Test: Update with empty condition type
	t.Run("InvalidArgument_EmptyConditionType", func(t *testing.T) {
		resp, err := svc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:      "gpu-0",
			Condition: &v1alpha1.Condition{Type: ""},
		})
		if resp != nil {
			t.Error("Expected nil response")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Test: Update non-existent GPU
	t.Run("NotFound", func(t *testing.T) {
		resp, err := svc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:      "gpu-missing",
			Condition: &v1alpha1.Condition{Type: "Ready", Status: "True"},
		})
		if resp != nil {
			t.Error("Expected nil response")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", st.Code())
		}
	})

	// Register a GPU with initial status
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-0"},
		&v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True"},
			},
		}, "test")

	// Test: Add new condition
	t.Run("AddNewCondition", func(t *testing.T) {
		resp, err := svc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:       "gpu-0",
			ProviderId: "health-monitor",
			Condition:  &v1alpha1.Condition{Type: "Healthy", Status: "True"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}

		// Verify condition was added
		gpu, _ := gpuCache.Get("gpu-0")
		if len(gpu.Status.Conditions) != 2 {
			t.Errorf("Expected 2 conditions, got %d", len(gpu.Status.Conditions))
		}
	})

	// Test: Update existing condition
	t.Run("UpdateExistingCondition", func(t *testing.T) {
		resp, err := svc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:       "gpu-0",
			ProviderId: "test",
			Condition:  &v1alpha1.Condition{Type: "Ready", Status: "False", Message: "GPU failed"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}

		// Verify condition was updated (not added)
		gpu, _ := gpuCache.Get("gpu-0")
		if len(gpu.Status.Conditions) != 2 {
			t.Errorf("Expected 2 conditions (updated, not added), got %d", len(gpu.Status.Conditions))
		}

		// Find the Ready condition
		var readyCondition *v1alpha1.Condition
		for _, c := range gpu.Status.Conditions {
			if c.Type == "Ready" {
				readyCondition = c
				break
			}
		}
		if readyCondition == nil {
			t.Fatal("Ready condition not found")
		}
		if readyCondition.Status != "False" {
			t.Errorf("Expected status=False, got %s", readyCondition.Status)
		}
		if readyCondition.Message != "GPU failed" {
			t.Errorf("Expected message='GPU failed', got %s", readyCondition.Message)
		}
	})
}

func TestProviderService_Heartbeat(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewProviderService(gpuCache)
	ctx := context.Background()

	// Heartbeat should return Unimplemented
	resp, err := svc.Heartbeat(ctx, &HeartbeatRequest{ProviderId: "test"})
	if resp != nil {
		t.Error("Expected nil response for unimplemented method")
	}
	if err == nil {
		t.Fatal("Expected error for unimplemented method")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("Expected Unimplemented, got %v", st.Code())
	}
}

func TestProviderService_WriteBlocksReads(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	providerSvc := NewProviderService(gpuCache)
	consumerSvc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Register a GPU
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-0"}, nil, "test")

	// This test verifies that write operations properly acquire locks.
	// The actual blocking behavior is tested in cache_test.go.
	// Here we just verify the services work correctly with the cache.

	// Provider updates status
	_, err := providerSvc.UpdateGpuStatus(ctx, &UpdateGpuStatusRequest{
		Name:       "gpu-0",
		ProviderId: "test",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Provider update failed: %v", err)
	}

	// Consumer reads should see the update
	resp, err := consumerSvc.GetGpu(ctx, &GetGpuRequest{Name: "gpu-0"})
	if err != nil {
		t.Fatalf("Consumer get failed: %v", err)
	}
	if len(resp.Gpu.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(resp.Gpu.Status.Conditions))
	}
	if resp.Gpu.Status.Conditions[0].Status != "True" {
		t.Errorf("Expected status=True, got %s", resp.Gpu.Status.Conditions[0].Status)
	}
}
