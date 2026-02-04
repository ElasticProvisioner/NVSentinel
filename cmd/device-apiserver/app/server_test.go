//  Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app/options"
	"github.com/nvidia/nvsentinel/pkg/util/testutils"
)

func TestRun(t *testing.T) {
	opts := options.NewServerRunOptions()

	localSocket := testutils.NewUnixAddr(t)
	kineSocket := fmt.Sprintf("unix://%s", testutils.NewUnixAddr(t))
	healthAddr := testutils.GetFreeTCPAddress(t)

	opts.GRPC.BindAddress = "unix://" + localSocket
	opts.HealthAddress = healthAddr
	opts.NodeName = "test-node"

	tmpDir := t.TempDir()
	opts.Storage.DatabaseDir = tmpDir
	opts.Storage.DatabasePath = tmpDir + "state.db"
	opts.Storage.KineSocketPath = kineSocket
	opts.Storage.KineConfig.Endpoint = fmt.Sprintf("sqlite://%s/db.sqlite", tmpDir)
	opts.Storage.KineConfig.Listener = kineSocket

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	completedOpts, err := opts.Complete(ctx)
	if err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, completedOpts)
	}()

	testutils.WaitForStatus(t, healthAddr, "", 5*time.Second, testutils.IsServing)

	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("exited with unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Failed to shut down within grace period")
	}

	if _, err := os.Stat(localSocket); err == nil {
		t.Errorf("socket file %q still exists after shutdown", localSocket)
	}
}

func TestRun_StorageFailure(t *testing.T) {
	opts := options.NewServerRunOptions()

	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0444); err != nil {
		t.Fatal(err)
	}

	opts.NodeName = "test-node"
	opts.Storage.DatabaseDir = readOnlyDir
	opts.Storage.DatabasePath = readOnlyDir + "state.db"
	opts.Storage.KineSocketPath = filepath.Join(readOnlyDir, "kine.sock")
	opts.Storage.KineConfig.Endpoint = fmt.Sprintf("sqlite://%s/db.sqlite", readOnlyDir)

	opts.HealthAddress = testutils.GetFreeTCPAddress(t)
	opts.GRPC.BindAddress = "unix://" + filepath.Join(tmpDir, "api.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	completedOpts, _ := opts.Complete(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, completedOpts)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Expected server to fail due to storage error, but it exited with nil")
		}
		if !strings.Contains(err.Error(), "storage") && !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("Expected storage or permission error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Server should have failed immediately on storage error, but it timed out/hung")
	}
}
