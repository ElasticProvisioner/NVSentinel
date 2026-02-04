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

package metrics

import (
	"testing"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

func TestServerMetrics_Register(t *testing.T) {
	m := &ServerMetrics{
		Registry: prometheus.NewRegistry(),
		Collectors: grpcprom.NewServerMetrics(
			grpcprom.WithServerHandlingTimeHistogram(),
		),
		ServiceHealthStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "device_apiserver_service_status",
				Help: "test health metric",
			},
			[]string{"service"},
		),
	}

	// Multiple calls should not panic or cause errors
	m.Register()
	m.Register()

	err := m.Registry.Register(m.Collectors)
	if err == nil {
		t.Error("expected error when re-registering collectors, but got nil")
	}
	if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
		t.Errorf("expected AlreadyRegisteredError, got %T: %v", err, err)
	}

	err = m.Registry.Register(m.ServiceHealthStatus)
	if err == nil {
		t.Error("expected error when re-registering health status metric, but got nil")
	}
}

func TestDefaultServerMetrics(t *testing.T) {
	if DefaultServerMetrics.Registry == nil {
		t.Fatal("DefaultServerMetrics.Registry should not be nil")
	}
	if DefaultServerMetrics.Collectors == nil {
		t.Fatal("DefaultServerMetrics.Collectors should not be nil")
	}
	if DefaultServerMetrics.ServiceHealthStatus == nil {
		t.Fatal("DefaultServerMetrics.ServiceHealthStatus should not be nil")
	}
}
