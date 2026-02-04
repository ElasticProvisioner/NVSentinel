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

package app

import (
	"context"
	"testing"

	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app/options"
)

func TestConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := options.NewServerRunOptions()

	completedOpts, err := opts.Complete(ctx)
	if err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	cfg, err := NewConfig(ctx, completedOpts)
	if err != nil {
		t.Fatalf("NewConfig failed: %v", err)
	}

	if cfg.Storage == nil {
		t.Error("NewConfig did not initialize Storage config")
	}
	if cfg.APIs == nil {
		t.Error("NewConfig did not initialize APIs config")
	}

	t.Run("Complete", func(t *testing.T) {
		completedCfg, err := cfg.Complete()
		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}

		if completedCfg.completedConfig == nil {
			t.Fatal("CompletedConfig internal pointer is nil")
		}

		validationErrors := completedCfg.Options.Validate()
		if len(validationErrors) > 0 {
			t.Errorf("CompletedConfig is invalid: %v", validationErrors)
		}
	})

	t.Run("NilSafety", func(t *testing.T) {
		var nilCfg *Config
		_, err := nilCfg.Complete()
		if err != nil {
			t.Errorf("Complete() on nil config should not return error, got: %v", err)
		}

		partialCfg := &Config{}
		_, err = partialCfg.Complete()
		if err != nil {
			t.Errorf("Complete() on empty config should handle nil sub-fields gracefully, got: %v", err)
		}
	})
}
