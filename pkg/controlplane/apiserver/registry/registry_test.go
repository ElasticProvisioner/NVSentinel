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
	"testing"

	"github.com/nvidia/nvsentinel/pkg/controlplane/apiserver/api"
	"google.golang.org/grpc"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

func TestRegistry(t *testing.T) {
	reset := func() {
		lock.Lock()
		providers = nil
		lock.Unlock()
	}

	t.Run("RegisterAndList", func(t *testing.T) {
		reset()

		sp1 := &fakeProvider{name: "test-service-1"}
		Register(sp1)

		sp2 := &fakeProvider{name: "test-service-2"}
		Register(sp2)

		svcProviders := List()
		if len(svcProviders) != 2 {
			t.Errorf("Expected 1 provider, got %d", len(svcProviders))
		}
		if svcProviders[0] != sp1 {
			t.Error("Returned provider does not match registered provider")
		}
	})

	t.Run("ConcurrentRegistration", func(t *testing.T) {
		reset()

		const count = 100
		var wg sync.WaitGroup
		wg.Add(count)

		for i := 0; i < count; i++ {
			go func() {
				defer wg.Done()
				Register(&fakeProvider{})
			}()
		}
		wg.Wait()

		actual := len(List())
		if actual != count {
			t.Errorf("Expected %d providers, got %d", count, actual)
		}
	})
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
