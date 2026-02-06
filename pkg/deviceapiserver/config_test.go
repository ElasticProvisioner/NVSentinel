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

package deviceapiserver

import (
	"strings"
	"testing"
)

func TestConfig_Validate_PortCollision(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GRPCAddress = ":50051"
	cfg.HealthPort = 8081
	cfg.MetricsPort = 8081 // same as health

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for port collision")
	}
	if !strings.Contains(err.Error(), "health-port") || !strings.Contains(err.Error(), "metrics-port") {
		t.Errorf("error should mention both ports, got: %v", err)
	}
}

func TestConfig_Validate_PortCollision_DefaultsPass(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}
