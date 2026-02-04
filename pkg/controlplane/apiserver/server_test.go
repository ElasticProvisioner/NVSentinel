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

package apiserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/k3s-io/kine/pkg/endpoint"
	"github.com/nvidia/nvsentinel/pkg/storage/storagebackend"
	"github.com/nvidia/nvsentinel/pkg/util/testutils"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestDeviceAPIServer(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	kineSocket := testutils.NewUnixAddr(t)
	apiSocket := testutils.NewUnixAddr(t)
	healthAddr := testutils.GetFreeTCPAddress(t)
	metricsAddr := testutils.GetFreeTCPAddress(t)

	storage := &storagebackend.Storage{
		KineSocketPath: "unix://" + kineSocket,
		DatabaseDir:    tmpDir,
		KineConfig: endpoint.Config{
			Listener:         "unix://" + kineSocket,
			Endpoint:         "sqlite://" + dbPath,
			CompactBatchSize: 100,
		},
	}

	s := &DeviceAPIServer{
		BindAddress:          "unix://" + apiSocket,
		HealthAddress:        healthAddr,
		ServiceMonitorPeriod: 100 * time.Millisecond,
		MetricsAddress:       metricsAddr,
		ShutdownGracePeriod:  1 * time.Second,
		Storage:              storage,
		DeviceServer:         grpc.NewServer(),
		AdminServer:          grpc.NewServer(),
	}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	ps, err := storage.PrepareRun(ctx)
	if err != nil {
		t.Fatalf("Failed to prepare storage: %v", err)
	}
	go ps.Run(ctx)

	// Wait for the kine socket to exist
	_ = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		return storage.IsReady(), nil
	})

	// Then start API server
	prepared, err := s.PrepareRun(ctx)
	if err != nil {
		t.Fatalf("PrepareRun failed: %v", err)
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- prepared.Run(ctx)
	}()

	testutils.WaitForStatus(t, healthAddr, "", 5*time.Second, testutils.IsServing)

	stop()

	select {
	case err := <-serverErr:
		if err != nil && err != context.Canceled && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("server exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server failed to shut down within grace period")
	}

	if _, err := os.Stat(apiSocket); !os.IsNotExist(err) {
		t.Error("socket file was not cleaned up")
	}
}
