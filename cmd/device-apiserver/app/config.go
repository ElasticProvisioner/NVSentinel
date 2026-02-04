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

	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app/options"
	controlplane "github.com/nvidia/nvsentinel/pkg/controlplane/apiserver"
	"github.com/nvidia/nvsentinel/pkg/storage/storagebackend"
)

type Config struct {
	Options options.CompletedOptions

	Storage *storagebackend.Config
	APIs    *controlplane.Config
}

type completedConfig struct {
	Options options.CompletedOptions

	Storage storagebackend.CompletedConfig
	APIs    controlplane.CompletedConfig
}

type CompletedConfig struct {
	*completedConfig
}

func NewConfig(ctx context.Context, opts options.CompletedOptions) (*Config, error) {
	c := &Config{
		Options: opts,
	}

	storageConfig, err := storagebackend.NewConfig(ctx, opts.Storage)
	if err != nil {
		return nil, err
	}

	c.Storage = storageConfig

	controlPlaneConfig, err := controlplane.NewConfig(ctx, opts.CompletedOptions)
	if err != nil {
		return nil, err
	}

	c.APIs = controlPlaneConfig

	return c, nil
}

func (c *Config) Complete() (CompletedConfig, error) {
	if c == nil || c.Storage == nil || c.APIs == nil {
		return CompletedConfig{}, nil
	}

	completedStorage, err := c.Storage.Complete()
	if err != nil {
		return CompletedConfig{}, err
	}

	completedAPIs, err := c.APIs.Complete()
	if err != nil {
		return CompletedConfig{}, err
	}

	return CompletedConfig{&completedConfig{
		Options: c.Options,

		Storage: completedStorage,
		APIs:    completedAPIs,
	}}, nil
}
