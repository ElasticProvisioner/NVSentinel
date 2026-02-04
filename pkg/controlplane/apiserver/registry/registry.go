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

package registry

import (
	"sync"

	"github.com/nvidia/nvsentinel/pkg/controlplane/apiserver/api"
)

var (
	lock      sync.RWMutex
	providers []api.ServiceProvider
)

// Register adds a service provider to the global registry. This function
// is typically called from the init() function of service implementation packages.
func Register(p api.ServiceProvider) {
	lock.Lock()
	defer lock.Unlock()

	providers = append(providers, p)
}

// List returns a copy of all currently registered service providers.
func List() []api.ServiceProvider {
	lock.RLock()
	defer lock.RUnlock()

	cp := make([]api.ServiceProvider, len(providers))
	copy(cp, providers)

	return cp
}
