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
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/nvsentinel/internal/generated/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/deviceapiserver/cache"
)

const bufSize = 1024 * 1024

// setupGRPCServer creates an in-process gRPC server and client for testing.
func setupGRPCServer(t *testing.T) (v1alpha1.GpuServiceClient, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()

	logger := klog.Background()
	gpuCache := cache.New(logger, nil)
	gpuService := NewGpuService(gpuCache)
	v1alpha1.RegisterGpuServiceServer(srv, gpuService)

	go func() {
		if err := srv.Serve(lis); err != nil {
			// Only log if not a graceful stop
			t.Logf("gRPC server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to create gRPC client: %v", err)
	}

	client := v1alpha1.NewGpuServiceClient(conn)

	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
	}

	return client, cleanup
}

func TestGRPC_CRUDCycle(t *testing.T) {
	client, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create
	gpu, err := client.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-grpc-0"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-GRPC-UUID"},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu over gRPC failed: %v", err)
	}
	if gpu.GetMetadata().GetName() != "gpu-grpc-0" {
		t.Errorf("Expected name=gpu-grpc-0, got %s", gpu.GetMetadata().GetName())
	}

	// Get
	resp, err := client.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-grpc-0"})
	if err != nil {
		t.Fatalf("GetGpu over gRPC failed: %v", err)
	}
	if resp.GetGpu().GetSpec().GetUuid() != "GPU-GRPC-UUID" {
		t.Errorf("Expected UUID GPU-GRPC-UUID, got %s", resp.GetGpu().GetSpec().GetUuid())
	}

	// UpdateGpuStatus
	updated, err := client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name: "gpu-grpc-0",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{
					Type:               "NVMLReady",
					Status:             "True",
					Reason:             "Healthy",
					Message:            "GPU is healthy",
					LastTransitionTime: timestamppb.Now(),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus over gRPC failed: %v", err)
	}
	if len(updated.GetStatus().GetConditions()) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(updated.GetStatus().GetConditions()))
	}

	// List
	listResp, err := client.ListGpus(ctx, &v1alpha1.ListGpusRequest{})
	if err != nil {
		t.Fatalf("ListGpus over gRPC failed: %v", err)
	}
	if len(listResp.GetGpuList().GetItems()) != 1 {
		t.Errorf("Expected 1 GPU, got %d", len(listResp.GetGpuList().GetItems()))
	}

	// Delete
	_, err = client.DeleteGpu(ctx, &v1alpha1.DeleteGpuRequest{Name: "gpu-grpc-0"})
	if err != nil {
		t.Fatalf("DeleteGpu over gRPC failed: %v", err)
	}

	// Verify deleted
	_, err = client.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: "gpu-grpc-0"})
	if err == nil {
		t.Fatal("Expected error for deleted GPU")
	}
	s, ok := status.FromError(err)
	if !ok || s.Code() != codes.NotFound {
		t.Errorf("Expected NotFound, got %v", err)
	}
}

func TestGRPC_ValidationErrors(t *testing.T) {
	client, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name     string
		do       func() error
		wantCode codes.Code
	}{
		{
			name:     "GetGpu empty name",
			do:       func() error { _, err := client.GetGpu(ctx, &v1alpha1.GetGpuRequest{Name: ""}); return err },
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "CreateGpu nil gpu",
			do:       func() error { _, err := client.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{}); return err },
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "DeleteGpu empty name",
			do:       func() error { _, err := client.DeleteGpu(ctx, &v1alpha1.DeleteGpuRequest{Name: ""}); return err },
			wantCode: codes.InvalidArgument,
		},
		{
			name: "UpdateGpuStatus empty name",
			do: func() error {
				_, err := client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
					Name:   "",
					Status: &v1alpha1.GpuStatus{},
				})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "UpdateGpuStatus nil status",
			do: func() error {
				_, err := client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
					Name: "gpu-0",
				})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "UpdateGpuStatus not found",
			do: func() error {
				_, err := client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
					Name:   "nonexistent",
					Status: &v1alpha1.GpuStatus{},
				})
				return err
			},
			wantCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.do()
			if err == nil {
				t.Fatal("Expected error")
			}
			s, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Expected gRPC status error, got %T: %v", err, err)
			}
			if s.Code() != tt.wantCode {
				t.Errorf("Expected code %v, got %v: %s", tt.wantCode, s.Code(), s.Message())
			}
		})
	}
}

func TestGRPC_OptimisticConcurrency(t *testing.T) {
	client, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create GPU
	gpu, err := client.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-occ"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-OCC"},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu failed: %v", err)
	}

	// UpdateGpuStatus with wrong version should fail
	_, err = client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name:            "gpu-occ",
		ResourceVersion: "999",
		Status:          &v1alpha1.GpuStatus{},
	})
	if err == nil {
		t.Fatal("Expected error for version conflict")
	}
	s, ok := status.FromError(err)
	if !ok || s.Code() != codes.Aborted {
		t.Errorf("Expected Aborted, got %v", err)
	}

	// UpdateGpuStatus with correct version should succeed
	_, err = client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name:            "gpu-occ",
		ResourceVersion: gpu.GetMetadata().GetResourceVersion(),
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "Ready", Status: "True"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus with correct version failed: %v", err)
	}
}

func TestGRPC_WatchGpus(t *testing.T) {
	client, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a GPU before starting the watch
	_, err := client.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-watch-existing"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-WATCH-EXISTING"},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu failed: %v", err)
	}

	// Start watch
	stream, err := client.WatchGpus(ctx, &v1alpha1.WatchGpusRequest{})
	if err != nil {
		t.Fatalf("WatchGpus failed: %v", err)
	}

	// Should receive ADDED event for existing GPU (initial sync)
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive initial event: %v", err)
	}
	if event.GetType() != "ADDED" {
		t.Errorf("Expected ADDED event, got %s", event.GetType())
	}
	if event.GetObject().GetMetadata().GetName() != "gpu-watch-existing" {
		t.Errorf("Expected gpu-watch-existing, got %s", event.GetObject().GetMetadata().GetName())
	}

	// Create another GPU — should trigger ADDED event
	_, err = client.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-watch-new"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-WATCH-NEW"},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu failed: %v", err)
	}

	event, err = stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive ADDED event: %v", err)
	}
	if event.GetType() != "ADDED" {
		t.Errorf("Expected ADDED event, got %s", event.GetType())
	}
	if event.GetObject().GetMetadata().GetName() != "gpu-watch-new" {
		t.Errorf("Expected gpu-watch-new, got %s", event.GetObject().GetMetadata().GetName())
	}

	// Update status — should trigger MODIFIED event
	_, err = client.UpdateGpuStatus(ctx, &v1alpha1.UpdateGpuStatusRequest{
		Name: "gpu-watch-new",
		Status: &v1alpha1.GpuStatus{
			Conditions: []*v1alpha1.Condition{
				{Type: "NVMLReady", Status: "True"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus failed: %v", err)
	}

	event, err = stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive MODIFIED event: %v", err)
	}
	if event.GetType() != "MODIFIED" {
		t.Errorf("Expected MODIFIED event, got %s", event.GetType())
	}

	// Delete — should trigger DELETED event
	_, err = client.DeleteGpu(ctx, &v1alpha1.DeleteGpuRequest{Name: "gpu-watch-new"})
	if err != nil {
		t.Fatalf("DeleteGpu failed: %v", err)
	}

	event, err = stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive DELETED event: %v", err)
	}
	if event.GetType() != "DELETED" {
		t.Errorf("Expected DELETED event, got %s", event.GetType())
	}
}

func TestGRPC_WatchGpus_CancelContext(t *testing.T) {
	client, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	stream, err := client.WatchGpus(ctx, &v1alpha1.WatchGpusRequest{})
	if err != nil {
		t.Fatalf("WatchGpus failed: %v", err)
	}

	// Cancel the context
	cancel()

	// Next recv should return an error (context cancelled or EOF)
	_, err = stream.Recv()
	if err == nil {
		t.Error("Expected error after context cancellation")
	}
}

func TestGRPC_WatchGpus_EmptyInitialSync(t *testing.T) {
	client, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start watch on empty cache
	stream, err := client.WatchGpus(ctx, &v1alpha1.WatchGpusRequest{})
	if err != nil {
		t.Fatalf("WatchGpus failed: %v", err)
	}

	// Create a GPU — should be the first event
	_, err = client.CreateGpu(ctx, &v1alpha1.CreateGpuRequest{
		Gpu: &v1alpha1.Gpu{
			Metadata: &v1alpha1.ObjectMeta{Name: "gpu-empty-sync"},
			Spec:     &v1alpha1.GpuSpec{Uuid: "GPU-EMPTY-SYNC"},
			Status:   &v1alpha1.GpuStatus{},
		},
	})
	if err != nil {
		t.Fatalf("CreateGpu failed: %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive event: %v", err)
	}
	if event.GetType() != "ADDED" {
		t.Errorf("Expected ADDED event, got %s", event.GetType())
	}

}
