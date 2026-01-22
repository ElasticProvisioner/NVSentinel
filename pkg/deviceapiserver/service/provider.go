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
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/nvidia/nvsentinel/api/gen/go/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/deviceapiserver/cache"
)

// ProviderService implements the ProviderService for health monitors (providers).
//
// This service provides write access to GPU resources. All write operations
// acquire an exclusive write lock on the cache, blocking ALL consumer read
// operations until the write completes.
//
// This blocking behavior is intentional to prevent consumers from reading
// stale "healthy" states when a GPU is transitioning to "unhealthy".
type ProviderService struct {
	v1alpha1.UnimplementedProviderServiceServer
	cache *cache.GpuCache
}

// NewProviderService creates a new ProviderService instance.
func NewProviderService(gpuCache *cache.GpuCache) *ProviderService {
	return &ProviderService{
		cache: gpuCache,
	}
}

// RegisterGpu registers a new GPU with the server.
//
// If the GPU already exists (same name), this returns the existing GPU's
// resource version without modification. Use UpdateGpuStatus to modify existing GPUs.
//
// This operation acquires a write lock, blocking all consumer reads.
func (s *ProviderService) RegisterGpu(ctx context.Context, req *RegisterGpuRequest) (*RegisterGpuResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"method", "RegisterGpu",
		"gpuName", req.GetName(),
		"providerID", req.GetProviderId(),
	)

	// Validate request
	if req.GetName() == "" {
		logger.V(1).Info("Invalid request: empty name")

		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Register GPU (acquires write lock, blocks consumers)
	created, resourceVersion, err := s.cache.Register(
		req.GetName(),
		req.GetSpec(),
		req.GetInitialStatus(),
		req.GetProviderId(),
	)
	if err != nil {
		logger.Error(err, "Failed to register GPU")

		return nil, status.Error(codes.Internal, "failed to register gpu")
	}

	if created {
		logger.Info("GPU registered", "resourceVersion", resourceVersion)
	} else {
		logger.V(1).Info("GPU already exists", "resourceVersion", resourceVersion)
	}

	return &RegisterGpuResponse{
		Created:         created,
		ResourceVersion: resourceVersion,
	}, nil
}

// UnregisterGpu removes a GPU from the server.
//
// After unregistration, the GPU will no longer appear in ListGpus or GetGpu
// responses. Active WatchGpus streams will receive a DELETED event.
//
// This operation acquires a write lock, blocking all consumer reads.
func (s *ProviderService) UnregisterGpu(
	ctx context.Context,
	req *UnregisterGpuRequest,
) (*UnregisterGpuResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"method", "UnregisterGpu",
		"gpuName", req.GetName(),
	)

	// Validate request
	if req.GetName() == "" {
		logger.V(1).Info("Invalid request: empty name")

		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Unregister GPU (acquires write lock, blocks consumers)
	deleted := s.cache.Unregister(req.GetName())

	if deleted {
		logger.Info("GPU unregistered")
	} else {
		logger.V(1).Info("GPU not found")
	}

	return &UnregisterGpuResponse{
		Deleted: deleted,
	}, nil
}

// UpdateGpuStatus replaces the entire status of a registered GPU.
//
// IMPORTANT: This operation acquires an exclusive write lock, blocking
// ALL consumer read operations (GetGpu, ListGpus) until the update completes.
// This ensures consumers never read stale data during status transitions.
//
// Returns NotFound if the GPU is not registered.
func (s *ProviderService) UpdateGpuStatus(
	ctx context.Context,
	req *UpdateGpuStatusRequest,
) (*UpdateGpuStatusResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"method", "UpdateGpuStatus",
		"gpuName", req.GetName(),
		"providerID", req.GetProviderId(),
	)

	// Validate request
	if req.GetName() == "" {
		logger.V(1).Info("Invalid request: empty name")

		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Update status (acquires write lock, BLOCKS ALL CONSUMER READS)
	resourceVersion, err := s.cache.UpdateStatus(
		req.GetName(),
		req.GetStatus(),
		req.GetProviderId(),
	)
	if err != nil {
		if errors.Is(err, cache.ErrGpuNotFound) {
			logger.V(1).Info("GPU not found")

			return nil, status.Error(codes.NotFound, "gpu not found")
		}

		logger.Error(err, "Failed to update GPU status")

		return nil, status.Error(codes.Internal, "failed to update status")
	}

	logger.V(1).Info("GPU status updated", "resourceVersion", resourceVersion)

	return &UpdateGpuStatusResponse{
		ResourceVersion: resourceVersion,
	}, nil
}

// UpdateGpuCondition updates or adds a single condition on a GPU.
//
// If a condition with the same type already exists, it is replaced.
// If no condition with that type exists, it is added.
//
// This is useful for providers that only manage specific condition types
// without affecting conditions managed by other providers.
//
// IMPORTANT: This operation acquires an exclusive write lock, blocking
// ALL consumer read operations until the update completes.
//
// Returns NotFound if the GPU is not registered.
func (s *ProviderService) UpdateGpuCondition(
	ctx context.Context,
	req *UpdateGpuConditionRequest,
) (*UpdateGpuConditionResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"method", "UpdateGpuCondition",
		"gpuName", req.GetName(),
		"conditionType", req.GetCondition().GetType(),
		"providerID", req.GetProviderId(),
	)

	// Validate request
	if req.GetName() == "" {
		logger.V(1).Info("Invalid request: empty name")

		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if req.GetCondition() == nil {
		logger.V(1).Info("Invalid request: nil condition")

		return nil, status.Error(codes.InvalidArgument, "condition is required")
	}

	if req.GetCondition().GetType() == "" {
		logger.V(1).Info("Invalid request: empty condition type")

		return nil, status.Error(codes.InvalidArgument, "condition type is required")
	}

	// Update condition (acquires write lock, BLOCKS ALL CONSUMER READS)
	resourceVersion, err := s.cache.UpdateCondition(
		req.GetName(),
		req.GetCondition(),
		req.GetProviderId(),
	)
	if err != nil {
		if errors.Is(err, cache.ErrGpuNotFound) {
			logger.V(1).Info("GPU not found")

			return nil, status.Error(codes.NotFound, "gpu not found")
		}

		logger.Error(err, "Failed to update GPU condition")

		return nil, status.Error(codes.Internal, "failed to update condition")
	}

	logger.V(1).Info("GPU condition updated",
		"conditionStatus", req.GetCondition().GetStatus(),
		"resourceVersion", resourceVersion,
	)

	return &UpdateGpuConditionResponse{
		ResourceVersion: resourceVersion,
	}, nil
}

// Heartbeat allows providers to indicate they are still active.
//
// NOTE: This is reserved for future implementation. The RPC is defined in the
// proto to avoid breaking changes when provider liveness detection is added.
// Currently returns Unimplemented.
func (s *ProviderService) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	logger := klog.FromContext(ctx).WithValues(
		"method", "Heartbeat",
		"providerID", req.GetProviderId(),
	)

	// Heartbeat is reserved for future provider liveness detection.
	// See proto documentation for planned behavior.
	logger.V(2).Info("Heartbeat received (reserved for future use)")

	return nil, status.Error(codes.Unimplemented,
		"heartbeat is reserved for future provider liveness detection; not yet implemented")
}

// Compile-time check that ProviderService implements ProviderServiceServer.
var _ v1alpha1.ProviderServiceServer = (*ProviderService)(nil)

// Type aliases for cleaner code (re-exported from generated code).
type (
	RegisterGpuRequest         = v1alpha1.RegisterGpuRequest
	RegisterGpuResponse        = v1alpha1.RegisterGpuResponse
	UnregisterGpuRequest       = v1alpha1.UnregisterGpuRequest
	UnregisterGpuResponse      = v1alpha1.UnregisterGpuResponse
	UpdateGpuStatusRequest     = v1alpha1.UpdateGpuStatusRequest
	UpdateGpuStatusResponse    = v1alpha1.UpdateGpuStatusResponse
	UpdateGpuConditionRequest  = v1alpha1.UpdateGpuConditionRequest
	UpdateGpuConditionResponse = v1alpha1.UpdateGpuConditionResponse
	HeartbeatRequest           = v1alpha1.HeartbeatRequest
	HeartbeatResponse          = v1alpha1.HeartbeatResponse
)
