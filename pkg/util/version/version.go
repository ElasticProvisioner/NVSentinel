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
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/component-base/compatibility"
)

var (
	GitVersion = "v0.0.0-devel"
	GitCommit  = "unknown"
	BuildDate  = "unknown"
)

type Info struct {
	GitVersion string
	GitCommit  string
	BuildDate  string
	GoVersion  string
	Compiler   string
	Platform   string
}

func Get() Info {
	return Info{
		GitVersion: GitVersion,
		GitCommit:  GitCommit,
		BuildDate:  BuildDate,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func (i Info) String() string {
	return i.GitVersion
}

// UserAgent returns the standard user agent string for clients.
func UserAgent() string {
	return fmt.Sprintf("nvidia-device-api/%s (%s)", GitVersion, Get().Platform)
}

func RegisterComponent(registry compatibility.ComponentGlobalsRegistry) error {
	v, err := utilversion.ParseSemantic(GitVersion)
	if err != nil {
		v = utilversion.MustParseSemantic("v0.0.1")
	}

	binaryVersion := v
	emulationVersion := v
	minCompatibilityVersion := v

	effectiveVer := compatibility.NewEffectiveVersion(
		binaryVersion,
		false,
		emulationVersion,
		minCompatibilityVersion,
	)

	if err := registry.Register("nvidia-device-api", effectiveVer, nil); err != nil {
		return fmt.Errorf("failed to register component with compatibility registry: %w", err)
	}

	return nil
}

func Handler() http.Handler {
	return http.HandlerFunc(versionHandler)
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Get())
}
