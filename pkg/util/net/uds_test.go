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

package net

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateUDSListener(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	ctx := context.Background()

	t.Run("CreateAndVerifyPermissions", func(t *testing.T) {
		_, cleanup, err := CreateUDSListener(ctx, socketPath, 0666)
		if err != nil {
			t.Fatalf("Failed to create listener: %v", err)
		}
		defer cleanup()

		info, err := os.Stat(socketPath)
		if err != nil {
			t.Fatalf("Socket file does not exist: %v", err)
		}

		if info.Mode().Perm() != 0666 {
			t.Errorf("Expected perms 0666, got %o", info.Mode().Perm())
		}
	})

	t.Run("RemoveStaleSocket", func(t *testing.T) {
		if err := os.WriteFile(socketPath, []byte("stale data"), 0666); err != nil {
			t.Fatalf("Failed to create fake stale socket: %v", err)
		}

		_, cleanup, err := CreateUDSListener(ctx, socketPath, 0666)
		if err != nil {
			t.Fatalf("Failed to recover from stale socket: %v", err)
		}
		defer cleanup()

		if _, err := net.Dial("unix", socketPath); err != nil {
			t.Errorf("Failed to dial the new socket: %v", err)
		}
	})
}

func TestCleanupUDS(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "cleanup.sock")

	t.Run("RemoveFile", func(t *testing.T) {
		if err := os.WriteFile(socketPath, []byte("data"), 0666); err != nil {
			t.Fatal(err)
		}

		CleanupUDS(socketPath)

		if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
			t.Error("Socket file should have been deleted")
		}
	})

	t.Run("IgnoreNonExistentFile", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "does-not-exist")
		CleanupUDS(nonExistentPath)
	})
}
