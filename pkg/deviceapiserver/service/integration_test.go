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
	"sync"
	"testing"
	"time"

	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/nvsentinel/api/gen/go/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/deviceapiserver/cache"
)

// TestIntegration_FullFlow tests the complete provider -> consumer flow:
// 1. Provider registers GPU
// 2. Consumer watches and receives ADDED event
// 3. Provider updates status
// 4. Consumer receives MODIFIED event
// 5. Provider unregisters GPU
// 6. Consumer receives DELETED event
func TestIntegration_FullFlow(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	providerSvc := NewProviderService(gpuCache)
	consumerSvc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Start watching before any GPUs exist
	stream := newMockWatchStream()
	var watchErr error
	watchDone := make(chan struct{})
	go func() {
		watchErr = consumerSvc.WatchGpus(&WatchGpusRequest{}, stream)
		close(watchDone)
	}()

	// Give watch time to start
	time.Sleep(50 * time.Millisecond)

	// Step 1: Provider registers GPU
	t.Log("Step 1: Provider registers GPU")
	regResp, err := providerSvc.RegisterGpu(ctx, &RegisterGpuRequest{
		Name:       "gpu-0",
		ProviderId: "test-provider",
		Spec:       &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
	})
	if err != nil {
		t.Fatalf("RegisterGpu failed: %v", err)
	}
	if !regResp.Created {
		t.Error("Expected Created=true")
	}

	// Wait for event
	time.Sleep(50 * time.Millisecond)

	// Step 2: Verify consumer received ADDED event
	t.Log("Step 2: Verify consumer received ADDED event")
	events := stream.getEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event (ADDED), got %d", len(events))
	} else if events[0].Type != cache.EventTypeAdded {
		t.Errorf("Expected ADDED event, got %s", events[0].Type)
	} else if events[0].Object.GetMetadata().GetName() != "gpu-0" {
		t.Errorf("Expected gpu-0, got %s", events[0].Object.GetMetadata().GetName())
	}

	// Step 3: Provider updates status
	t.Log("Step 3: Provider updates status")
	_, err = providerSvc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
		Name:       "gpu-0",
		ProviderId: "test-provider",
		Condition:  &v1alpha1.Condition{Type: "Ready", Status: "True", Message: "GPU is ready"},
	})
	if err != nil {
		t.Fatalf("UpdateGpuCondition failed: %v", err)
	}

	// Wait for event
	time.Sleep(50 * time.Millisecond)

	// Step 4: Verify consumer received MODIFIED event
	t.Log("Step 4: Verify consumer received MODIFIED event")
	events = stream.getEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 events (ADDED + MODIFIED), got %d", len(events))
	} else if events[1].Type != cache.EventTypeModified {
		t.Errorf("Expected MODIFIED event, got %s", events[1].Type)
	}

	// Verify condition through GetGpu
	getResp, err := consumerSvc.GetGpu(ctx, &GetGpuRequest{Name: "gpu-0"})
	if err != nil {
		t.Fatalf("GetGpu failed: %v", err)
	}
	if len(getResp.Gpu.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(getResp.Gpu.Status.Conditions))
	} else if getResp.Gpu.Status.Conditions[0].Status != "True" {
		t.Errorf("Expected status=True, got %s", getResp.Gpu.Status.Conditions[0].Status)
	}

	// Step 5: Provider unregisters GPU
	t.Log("Step 5: Provider unregisters GPU")
	unregResp, err := providerSvc.UnregisterGpu(ctx, &UnregisterGpuRequest{Name: "gpu-0"})
	if err != nil {
		t.Fatalf("UnregisterGpu failed: %v", err)
	}
	if !unregResp.Deleted {
		t.Error("Expected Deleted=true")
	}

	// Wait for event
	time.Sleep(50 * time.Millisecond)

	// Step 6: Verify consumer received DELETED event
	t.Log("Step 6: Verify consumer received DELETED event")
	events = stream.getEvents()
	if len(events) != 3 {
		t.Errorf("Expected 3 events (ADDED + MODIFIED + DELETED), got %d", len(events))
	} else if events[2].Type != cache.EventTypeDeleted {
		t.Errorf("Expected DELETED event, got %s", events[2].Type)
	}

	// Verify GPU is gone via ListGpus
	listResp, err := consumerSvc.ListGpus(ctx, &ListGpusRequest{})
	if err != nil {
		t.Fatalf("ListGpus failed: %v", err)
	}
	if len(listResp.GpuList.Items) != 0 {
		t.Errorf("Expected 0 GPUs, got %d", len(listResp.GpuList.Items))
	}

	// Cleanup
	stream.cancel()
	<-watchDone

	if watchErr != nil {
		t.Errorf("Watch error: %v", watchErr)
	}
}

// TestIntegration_MultipleProviders tests multiple providers updating the same GPU
func TestIntegration_MultipleProviders(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	providerSvc := NewProviderService(gpuCache)
	consumerSvc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Provider 1 registers GPU
	_, err := providerSvc.RegisterGpu(ctx, &RegisterGpuRequest{
		Name:       "gpu-0",
		ProviderId: "device-plugin",
		Spec:       &v1alpha1.GpuSpec{Uuid: "GPU-1234"},
		InitialStatus: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "DeviceReady", Status: "True"},
			},
		},
	})
	if err != nil {
		t.Fatalf("RegisterGpu failed: %v", err)
	}

	// Provider 2 adds a different condition
	_, err = providerSvc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
		Name:       "gpu-0",
		ProviderId: "health-monitor",
		Condition:  &v1alpha1.Condition{Type: "NVMLReady", Status: "True"},
	})
	if err != nil {
		t.Fatalf("UpdateGpuCondition (health-monitor) failed: %v", err)
	}

	// Verify both conditions exist
	resp, err := consumerSvc.GetGpu(ctx, &GetGpuRequest{Name: "gpu-0"})
	if err != nil {
		t.Fatalf("GetGpu failed: %v", err)
	}

	conditionTypes := make(map[string]string)
	for _, c := range resp.Gpu.Status.Conditions {
		conditionTypes[c.Type] = c.Status
	}

	if len(conditionTypes) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditionTypes))
	}
	if conditionTypes["DeviceReady"] != "True" {
		t.Errorf("Expected DeviceReady=True, got %s", conditionTypes["DeviceReady"])
	}
	if conditionTypes["NVMLReady"] != "True" {
		t.Errorf("Expected NVMLReady=True, got %s", conditionTypes["NVMLReady"])
	}
}

// TestIntegration_ConcurrentProvidersAndConsumers tests concurrent access patterns
func TestIntegration_ConcurrentProvidersAndConsumers(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	providerSvc := NewProviderService(gpuCache)
	consumerSvc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Register some GPUs
	for i := 0; i < 5; i++ {
		_, err := providerSvc.RegisterGpu(ctx, &RegisterGpuRequest{
			Name:       "gpu-" + string(rune('0'+i)),
			ProviderId: "test",
			Spec:       &v1alpha1.GpuSpec{Uuid: "GPU"},
		})
		if err != nil {
			t.Fatalf("RegisterGpu failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	// Start multiple concurrent providers updating conditions
	for p := 0; p < 5; p++ {
		wg.Add(1)
		go func(providerID int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				gpuName := "gpu-" + string(rune('0'+(i%5)))
				_, err := providerSvc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
					Name:       gpuName,
					ProviderId: "provider-" + string(rune('0'+providerID)),
					Condition: &v1alpha1.Condition{
						Type:   "TestCondition",
						Status: "True",
					},
				})
				if err != nil {
					errors <- err
				}
			}
		}(p)
	}

	// Start multiple concurrent consumers reading
	for c := 0; c < 10; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, err := consumerSvc.ListGpus(ctx, &ListGpusRequest{})
				if err != nil {
					errors <- err
				}
				gpuName := "gpu-" + string(rune('0'+(i%5)))
				_, _ = consumerSvc.GetGpu(ctx, &GetGpuRequest{Name: gpuName})
			}
		}()
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Total errors: %d", errorCount)
	}
}

// TestIntegration_WatchWithHighUpdateRate tests watch under high update rates
func TestIntegration_WatchWithHighUpdateRate(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	providerSvc := NewProviderService(gpuCache)
	consumerSvc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Register a GPU
	_, err := providerSvc.RegisterGpu(ctx, &RegisterGpuRequest{
		Name:       "gpu-0",
		ProviderId: "test",
		Spec:       &v1alpha1.GpuSpec{Uuid: "GPU-0"},
	})
	if err != nil {
		t.Fatalf("RegisterGpu failed: %v", err)
	}

	// Start watching
	stream := newMockWatchStream()
	watchDone := make(chan struct{})
	go func() {
		_ = consumerSvc.WatchGpus(&WatchGpusRequest{}, stream)
		close(watchDone)
	}()

	// Wait for watch to start
	time.Sleep(50 * time.Millisecond)

	// Send rapid updates
	const numUpdates = 100
	for i := 0; i < numUpdates; i++ {
		_, err := providerSvc.UpdateGpuCondition(ctx, &UpdateGpuConditionRequest{
			Name:       "gpu-0",
			ProviderId: "test",
			Condition: &v1alpha1.Condition{
				Type:    "TestCondition",
				Status:  "True",
				Message: "Update " + string(rune('0'+i%10)),
			},
		})
		if err != nil {
			t.Fatalf("UpdateGpuCondition failed at %d: %v", i, err)
		}
	}

	// Wait for events to propagate
	time.Sleep(100 * time.Millisecond)

	// Cancel watch
	stream.cancel()
	<-watchDone

	events := stream.getEvents()

	// Should have at least 1 initial ADDED + some MODIFIED events
	// Note: Some events may be dropped if buffer is full (expected behavior)
	if len(events) < 2 {
		t.Errorf("Expected at least 2 events, got %d", len(events))
	}

	t.Logf("Received %d events out of %d updates", len(events)-1, numUpdates)
}
