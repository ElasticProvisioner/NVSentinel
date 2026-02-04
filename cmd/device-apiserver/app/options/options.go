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

package options

import (
	"context"

	cp "github.com/nvidia/nvsentinel/pkg/controlplane/apiserver/options"
	cliflag "k8s.io/component-base/cli/flag"
)

type ServerRunOptions struct {
	*cp.Options
}

type completedOptions struct {
	cp.CompletedOptions
}

type CompletedOptions struct {
	*completedOptions
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{
		Options: cp.NewOptions(),
	}
}

func (s *ServerRunOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	if s == nil || s.Options == nil {
		return fss
	}

	s.AddFlags(&fss)

	return fss
}

func (o *ServerRunOptions) Complete(ctx context.Context) (CompletedOptions, error) {
	if o == nil {
		return CompletedOptions{completedOptions: &completedOptions{}}, nil
	}

	controlplane, err := o.Options.Complete(ctx)
	if err != nil {
		return CompletedOptions{}, err
	}

	completed := completedOptions{
		CompletedOptions: controlplane,
	}

	return CompletedOptions{
		completedOptions: &completed,
	}, nil
}

func (o completedOptions) Validate() []error {
	errs := o.CompletedOptions.Validate()

	return errs
}
