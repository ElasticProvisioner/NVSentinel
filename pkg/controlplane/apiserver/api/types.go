//  Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package api

import (
	"google.golang.org/grpc"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

// ServiceProvider defines the interface for components that can connect
// to a storage backend and install themselves onto a gRPC server.
type ServiceProvider interface {
	// Install initializes the service with the provided storage configuration
	// and registers its handlers with the gRPC server.
	Install(svr *grpc.Server, storage storagebackend.Config) (Service, error)
}

// Service represents a running API service managed by the DeviceAPIServer.
type Service interface {
	// Name returns the unique identifier for the service.
	Name() string
	// IsReady returns true if the service is currently capable of handling requests.
	IsReady() bool
	// Cleanup performs a graceful shutdown of th service, releasing any internal resources.
	Cleanup()
}
