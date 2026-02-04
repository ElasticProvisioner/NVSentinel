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

package apiserver

import (
	"context"
	"testing"
	"time"

	"github.com/nvidia/nvsentinel/pkg/controlplane/apiserver/api"
	"github.com/nvidia/nvsentinel/pkg/controlplane/apiserver/options"
	"github.com/nvidia/nvsentinel/pkg/controlplane/apiserver/registry"
	"google.golang.org/grpc"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

func TestNewConfig(t *testing.T) {
	ctx := context.Background()

	registry.Register(&fakeProvider{})

	opts := options.NewOptions()
	opts.NodeName = "test-apiserver"
	completedOpts, err := opts.Complete(ctx)
	if err != nil {
		t.Fatalf("failed to complete options: %v", err)
	}

	config, err := NewConfig(ctx, completedOpts)
	if err != nil {
		t.Fatalf("NewConfig failed: %v", err)
	}

	if config.NodeName != "test-apiserver" {
		t.Errorf("NodeName mismatch: got %s", config.NodeName)
	}
	if config.HealthAddress == "" || config.MetricsAddress == "" {
		t.Errorf("default addresses were not populated")
	}
	if config.ServiceMonitorPeriod != 10*time.Second {
		t.Errorf("expected service monitor period %s, got %s", 10*time.Second, config.ServiceMonitorPeriod)
	}
	if config.ShutdownGracePeriod != 25*time.Second {
		t.Errorf("expected shutdown grace period %s, got %s", 25*time.Second, config.ShutdownGracePeriod)
	}
	if len(config.ServerOptions) < 2 {
		t.Errorf("expected at least 2 gRPC interceptors, got %d", len(config.ServerOptions))
	}
	if len(config.ServiceProviders) != 1 {
		t.Errorf("expected 1 service provider, got %d", len(config.ServiceProviders))
	}
}

func TestComplete(t *testing.T) {
	var c *Config
	completed, err := c.Complete()
	if err != nil || completed.Config != nil {
		t.Errorf("Complete() on nil should return empty wrap, got: %v", completed)
	}

	c = &Config{NodeName: "ready"}
	completed, _ = c.Complete()
	if completed.NodeName != "ready" {
		t.Error("Complete() failed to wrap existing config")
	}
}

// fakeProvider satisfies api.ServiceProvider
type fakeProvider struct {
	name string
}

func (f *fakeProvider) Install(svr *grpc.Server, storage storagebackend.Config) (api.Service, error) {
	return &fakeService{name: f.name}, nil
}

// fakeService satisfies api.Service
type fakeService struct {
	name string
}

func (s *fakeService) Name() string  { return s.name }
func (s *fakeService) IsReady() bool { return true }
func (s *fakeService) Cleanup()      {}
