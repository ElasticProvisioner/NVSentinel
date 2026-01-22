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

// Package service implements the gRPC services for the Device API Server.
package service

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/nvsentinel/api/gen/go/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/deviceapiserver/cache"
)

// ConsumerService implements the GpuService for consumers (device plugins, DRA drivers).
//
// This service provides read-only access to GPU resources. All read operations
// acquire a read lock on the cache, which will block if a provider is currently
// updating GPU state. This ensures consumers never read stale data.
type ConsumerService struct {
	v1alpha1.UnimplementedGpuServiceServer
	cache *cache.GpuCache
}

// NewConsumerService creates a new ConsumerService instance.
func NewConsumerService(gpuCache *cache.GpuCache) *ConsumerService {
	return &ConsumerService{
		cache: gpuCache,
	}
}

// GetGpu retrieves a single GPU resource by its unique name.
//
// This method acquires a read lock on the cache, blocking if a write is in progress.
// Returns NotFound if the GPU does not exist.
func (s *ConsumerService) GetGpu(ctx context.Context, req *GetGpuRequest) (*GetGpuResponse, error) {
	logger := klog.FromContext(ctx).WithValues("method", "GetGpu", "gpuName", req.GetName())

	// Validate request
	if req.GetName() == "" {
		logger.V(1).Info("Invalid request: empty name")

		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Get GPU from cache (acquires read lock, blocks during writes)
	gpu, found := s.cache.Get(req.GetName())
	if !found {
		logger.V(1).Info("GPU not found")

		return nil, status.Error(codes.NotFound, "gpu not found")
	}

	logger.V(2).Info("GPU retrieved successfully", "resourceVersion", gpu.GetResourceVersion())

	return &GetGpuResponse{Gpu: gpu}, nil
}

// ListGpus retrieves a list of all GPU resources.
//
// This method acquires a read lock on the cache, blocking if a write is in progress.
// Returns an empty list if no GPUs are registered.
//
// The response includes ListMeta.ResourceVersion which can be used for
// optimistic concurrency or cache invalidation.
func (s *ConsumerService) ListGpus(ctx context.Context, req *ListGpusRequest) (*ListGpusResponse, error) {
	logger := klog.FromContext(ctx).WithValues("method", "ListGpus")

	// Get resource version first (under read lock)
	resourceVersion := s.cache.ResourceVersion()

	// List all GPUs from cache (acquires read lock, blocks during writes)
	gpus := s.cache.List()

	logger.V(2).Info("GPUs listed successfully",
		"count", len(gpus),
		"resourceVersion", resourceVersion,
	)

	return &ListGpusResponse{
		GpuList: &v1alpha1.GpuList{
			Metadata: &v1alpha1.ListMeta{
				ResourceVersion: strconv.FormatInt(resourceVersion, 10),
			},
			Items: gpus,
		},
	}, nil
}

// WatchGpus streams lifecycle events for GPU resources.
//
// The stream first sends ADDED events for all existing GPUs (initial sync),
// then streams MODIFIED and DELETED events as they occur.
//
// The stream remains open until:
//   - The client disconnects
//   - The server shuts down
//   - An error occurs
func (s *ConsumerService) WatchGpus(req *WatchGpusRequest, stream grpc.ServerStreamingServer[WatchGpusResponse]) error {
	ctx := stream.Context()
	logger := klog.FromContext(ctx).WithValues("method", "WatchGpus")

	// Generate unique subscriber ID
	subscriberID := uuid.New().String()
	logger = logger.WithValues("subscriberID", subscriberID)
	logger.V(1).Info("Watch stream started")

	// Subscribe to cache events
	broadcaster := s.cache.Broadcaster()
	events := broadcaster.Subscribe(subscriberID)

	defer func() {
		broadcaster.Unsubscribe(subscriberID)
		logger.V(1).Info("Watch stream ended")
	}()

	// Send initial state (all existing GPUs as ADDED events)
	gpus := s.cache.List()

	for _, gpu := range gpus {
		if err := stream.Send(&WatchGpusResponse{
			Type:   cache.EventTypeAdded,
			Object: gpu,
		}); err != nil {
			logger.Error(err, "Failed to send initial GPU state")

			return status.Error(codes.Internal, "failed to send initial state")
		}
	}

	logger.V(2).Info("Sent initial state", "gpuCount", len(gpus))

	// Stream ongoing events
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled
			logger.V(1).Info("Watch stream context done", "reason", ctx.Err())
			return nil

		case event, ok := <-events:
			if !ok {
				// Channel closed (server shutting down)
				logger.V(1).Info("Event channel closed")
				return nil
			}

			// Send event to client
			if err := stream.Send(&WatchGpusResponse{
				Type:   event.Type,
				Object: event.Object,
			}); err != nil {
				logger.Error(err, "Failed to send event",
					"eventType", event.Type,
					"gpuName", event.Object.GetMetadata().GetName(),
				)

				return status.Error(codes.Internal, "failed to send event")
			}

			logger.V(2).Info("Event sent",
				"eventType", event.Type,
				"gpuName", event.Object.GetMetadata().GetName(),
				"resourceVersion", event.Object.GetResourceVersion(),
			)
		}
	}
}

// Compile-time check that ConsumerService implements GpuServiceServer.
var _ v1alpha1.GpuServiceServer = (*ConsumerService)(nil)

// Type aliases for cleaner code (re-exported from generated code).
type (
	GetGpuRequest     = v1alpha1.GetGpuRequest
	GetGpuResponse    = v1alpha1.GetGpuResponse
	ListGpusRequest   = v1alpha1.ListGpusRequest
	ListGpusResponse  = v1alpha1.ListGpusResponse
	WatchGpusRequest  = v1alpha1.WatchGpusRequest
	WatchGpusResponse = v1alpha1.WatchGpusResponse
)
