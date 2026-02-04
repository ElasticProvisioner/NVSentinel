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

package v1alpha1_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app"
	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app/options"
	"github.com/nvidia/nvsentinel/pkg/client-go/client/versioned"
	"github.com/nvidia/nvsentinel/pkg/grpc/client"
	"github.com/nvidia/nvsentinel/pkg/util/testutils"
)

var (
	clientset    versioned.Interface
	serverCtx    context.Context
	serverCancel context.CancelFunc
)

func TestMain(m *testing.M) {
	serverCtx, serverCancel = context.WithCancel(context.Background())
	defer serverCancel()

	tmpDir, _ := os.MkdirTemp("", "nvsentinel-test-*")
	defer os.RemoveAll(tmpDir)

	socketPath, socketDir, err := testutils.CreateUnixAddr()
	if err != nil {
		fmt.Printf("Failed to create socket path: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(socketDir)

	kineSocketPath, kineDir, err := testutils.CreateUnixAddr()
	if err != nil {
		fmt.Printf("Failed to create kine socket path: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(kineDir)

	healthAddr, err := testutils.GetFreeTCPAddress()
	if err != nil {
		fmt.Printf("Failed to get free TCP port: %v\n", err)
		os.Exit(1)
	}

	opts := options.NewServerRunOptions()
	opts.NodeName = "test-node"
	opts.GRPC.BindAddress = "unix://" + socketPath
	opts.HealthAddress = healthAddr
	opts.Storage.DatabaseDir = tmpDir
	opts.Storage.DatabasePath = tmpDir + "/state.db"
	opts.Storage.KineSocketPath = kineSocketPath
	opts.Storage.KineConfig.Endpoint = fmt.Sprintf("sqlite://%s/db.sqlite", tmpDir)
	opts.Storage.KineConfig.Listener = "unix://" + kineSocketPath

	completed, err := opts.Complete(serverCtx)
	if err != nil {
		fmt.Printf("Failed to complete options: %v\n", err)
		os.Exit(1)
	}

	go func() {
		if err := app.Run(serverCtx, completed); err != nil && err != context.Canceled {
			fmt.Printf("Server exited with error: %v\n", err)
		}
	}()

	err = testutils.PollHealthStatus(healthAddr, "", 5*time.Second, testutils.IsServing)
	if err != nil {
		fmt.Printf("Server timed out waiting for health check: %v\n", err)
		os.Exit(1)
	}

	config := &client.Config{Target: "unix://" + socketPath}
	clientset, err = versioned.NewForConfig(config)
	if err != nil {
		fmt.Printf("Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	serverCancel()
	os.Exit(code)
}
