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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/device-api/api/gen/go/device/v1alpha1"
	"github.com/nvidia/device-api/pkg/deviceapiserver/cache"
)

func TestConsumerService_GetGpu(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Test: Get non-existent GPU
	t.Run("NotFound", func(t *testing.T) {
		resp, err := svc.GetGpu(ctx, &GetGpuRequest{Name: "gpu-missing"})
		if resp != nil {
			t.Error("Expected nil response for missing GPU")
		}
		if err == nil {
			t.Fatal("Expected error for missing GPU")
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", st.Code())
		}
	})

	// Test: Get with empty name
	t.Run("InvalidArgument", func(t *testing.T) {
		resp, err := svc.GetGpu(ctx, &GetGpuRequest{Name: ""})
		if resp != nil {
			t.Error("Expected nil response for empty name")
		}
		if err == nil {
			t.Fatal("Expected error for empty name")
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", st.Code())
		}
	})

	// Register a GPU
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-1234"}, nil, "test-provider")

	// Test: Get existing GPU
	t.Run("Success", func(t *testing.T) {
		resp, err := svc.GetGpu(ctx, &GetGpuRequest{Name: "gpu-0"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		if resp.Gpu == nil {
			t.Fatal("Expected non-nil GPU in response")
		}
		if resp.Gpu.GetMetadata().GetName() != "gpu-0" {
			t.Errorf("Expected name=gpu-0, got %s", resp.Gpu.GetMetadata().GetName())
		}
		if resp.Gpu.Spec.Uuid != "GPU-1234" {
			t.Errorf("Expected UUID=GPU-1234, got %s", resp.Gpu.Spec.Uuid)
		}
	})
}

func TestConsumerService_ListGpus(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Test: Empty list
	t.Run("EmptyList", func(t *testing.T) {
		resp, err := svc.ListGpus(ctx, &ListGpusRequest{})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil || resp.GpuList == nil {
			t.Fatal("Expected non-nil response and GpuList")
		}
		if len(resp.GpuList.Items) != 0 {
			t.Errorf("Expected empty list, got %d items", len(resp.GpuList.Items))
		}
	})

	// Register some GPUs
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-0"}, nil, "test")
	gpuCache.Register("gpu-1", &v1alpha1.GpuSpec{Uuid: "GPU-1"}, nil, "test")
	gpuCache.Register("gpu-2", &v1alpha1.GpuSpec{Uuid: "GPU-2"}, nil, "test")

	// Test: List with GPUs
	t.Run("WithGpus", func(t *testing.T) {
		resp, err := svc.ListGpus(ctx, &ListGpusRequest{})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp == nil || resp.GpuList == nil {
			t.Fatal("Expected non-nil response and GpuList")
		}
		if len(resp.GpuList.Items) != 3 {
			t.Errorf("Expected 3 GPUs, got %d", len(resp.GpuList.Items))
		}
	})
}

// mockWatchStream implements grpc.ServerStreamingServer for testing WatchGpus.
type mockWatchStream struct {
	grpc.ServerStream
	ctx      context.Context
	cancel   context.CancelFunc
	events   []*WatchGpusResponse
	eventsMu sync.Mutex
	sendErr  error
}

func newMockWatchStream() *mockWatchStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockWatchStream{
		ctx:    ctx,
		cancel: cancel,
		events: make([]*WatchGpusResponse, 0),
	}
}

func (m *mockWatchStream) Send(resp *WatchGpusResponse) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.eventsMu.Lock()
	m.events = append(m.events, resp)
	m.eventsMu.Unlock()
	return nil
}

func (m *mockWatchStream) Context() context.Context {
	return m.ctx
}

func (m *mockWatchStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockWatchStream) SendHeader(metadata.MD) error { return nil }
func (m *mockWatchStream) SetTrailer(metadata.MD)       {}
func (m *mockWatchStream) SendMsg(any) error            { return nil }
func (m *mockWatchStream) RecvMsg(any) error            { return nil }

func (m *mockWatchStream) getEvents() []*WatchGpusResponse {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	result := make([]*WatchGpusResponse, len(m.events))
	copy(result, m.events)
	return result
}

func TestConsumerService_WatchGpus(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewConsumerService(gpuCache)

	// Register some GPUs before watching
	gpuCache.Register("gpu-0", &v1alpha1.GpuSpec{Uuid: "GPU-0"}, nil, "test")
	gpuCache.Register("gpu-1", &v1alpha1.GpuSpec{Uuid: "GPU-1"}, nil, "test")

	t.Run("InitialSync", func(t *testing.T) {
		stream := newMockWatchStream()

		// Start watch in goroutine
		var watchErr error
		watchDone := make(chan struct{})
		go func() {
			watchErr = svc.WatchGpus(&WatchGpusRequest{}, stream)
			close(watchDone)
		}()

		// Wait for initial events to be sent
		time.Sleep(50 * time.Millisecond)

		// Cancel stream
		stream.cancel()
		<-watchDone

		if watchErr != nil {
			t.Errorf("Unexpected watch error: %v", watchErr)
		}

		events := stream.getEvents()
		if len(events) != 2 {
			t.Errorf("Expected 2 initial events (ADDED), got %d", len(events))
		}

		// All initial events should be ADDED
		for _, e := range events {
			if e.Type != cache.EventTypeAdded {
				t.Errorf("Expected ADDED event, got %s", e.Type)
			}
		}
	})

	t.Run("StreamUpdates", func(t *testing.T) {
		stream := newMockWatchStream()

		// Start watch in goroutine
		var watchErr error
		watchDone := make(chan struct{})
		go func() {
			watchErr = svc.WatchGpus(&WatchGpusRequest{}, stream)
			close(watchDone)
		}()

		// Wait for initial sync
		time.Sleep(50 * time.Millisecond)

		// Register a new GPU - should trigger ADDED event
		gpuCache.Register("gpu-2", &v1alpha1.GpuSpec{Uuid: "GPU-2"}, nil, "test")

		// Wait for event to be sent
		time.Sleep(50 * time.Millisecond)

		// Update condition - should trigger MODIFIED event
		gpuCache.UpdateCondition("gpu-2", &v1alpha1.Condition{Type: "Ready", Status: "True"}, "test")

		// Wait for event
		time.Sleep(50 * time.Millisecond)

		// Cancel stream
		stream.cancel()
		<-watchDone

		if watchErr != nil {
			t.Errorf("Unexpected watch error: %v", watchErr)
		}

		events := stream.getEvents()
		// Should have: 2 initial ADDED + 1 new ADDED + 1 MODIFIED = 4
		if len(events) < 4 {
			t.Errorf("Expected at least 4 events, got %d", len(events))
		}

		// Find the MODIFIED event
		hasModified := false
		for _, e := range events {
			if e.Type == cache.EventTypeModified {
				hasModified = true
				break
			}
		}
		if !hasModified {
			t.Error("Expected at least one MODIFIED event")
		}
	})

	t.Run("ClientDisconnect", func(t *testing.T) {
		stream := newMockWatchStream()

		watchDone := make(chan struct{})
		go func() {
			_ = svc.WatchGpus(&WatchGpusRequest{}, stream)
			close(watchDone)
		}()

		// Wait for watch to start
		time.Sleep(20 * time.Millisecond)

		// Cancel context (simulate client disconnect)
		stream.cancel()

		// Watch should exit gracefully
		select {
		case <-watchDone:
			// Good - watch exited
		case <-time.After(time.Second):
			t.Error("Watch did not exit after client disconnect")
		}
	})
}

func TestConsumerService_ConcurrentReads(t *testing.T) {
	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	svc := NewConsumerService(gpuCache)
	ctx := context.Background()

	// Register GPUs
	for i := 0; i < 10; i++ {
		gpuCache.Register("gpu-"+string(rune('0'+i)), &v1alpha1.GpuSpec{Uuid: "GPU"}, nil, "test")
	}

	// Concurrent reads should all succeed
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				resp, err := svc.ListGpus(ctx, &ListGpusRequest{})
				if err != nil {
					errors <- err
					return
				}
				if len(resp.GpuList.Items) != 10 {
					errors <- status.Errorf(codes.Internal, "expected 10 GPUs, got %d", len(resp.GpuList.Items))
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent read error: %v", err)
	}
}
