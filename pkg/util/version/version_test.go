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

package version

import (
	"strings"
	"testing"

	"k8s.io/component-base/compatibility"
)

func TestGet(t *testing.T) {
	info := Get()

	if info.GitVersion != GitVersion {
		t.Errorf("expected GitVersion %s, got %s", GitVersion, info.GitVersion)
	}

	if info.GoVersion == "" || info.Platform == "" {
		t.Error("runtime info (GoVersion/Platform) should not be empty")
	}
}

func TestUserAgent(t *testing.T) {
	ua := UserAgent()
	expectedPrefix := "nvidia-device-api/" + GitVersion

	if !strings.HasPrefix(ua, expectedPrefix) {
		t.Errorf("UserAgent %s does not start with %s", ua, expectedPrefix)
	}
}

func TestRegisterComponent(t *testing.T) {
	tests := []struct {
		name       string
		gitVersion string
	}{
		{
			name:       "valid semver",
			gitVersion: "v1.2.3",
		},
		{
			name:       "invalid semver uses fallback",
			gitVersion: "development-build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVersion := GitVersion
			GitVersion = tt.gitVersion
			defer func() { GitVersion = oldVersion }()

			registry := compatibility.NewComponentGlobalsRegistry()

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("RegisterComponent panicked for version %s: %v", tt.gitVersion, r)
				}
			}()

			RegisterComponent(registry)

			effective := registry.EffectiveVersionFor("nvidia-device-api")
			if effective == nil {
				t.Fatal("component was not registered in the registry")
			}

			if effective.BinaryVersion() == nil {
				t.Error("EffectiveVersion has nil BinaryVersion")
			}
		})
	}
}
