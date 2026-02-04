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

package testutils

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// CreateUnixAddr creates a temporary directory and returns a socket path and the directory.
func CreateUnixAddr() (string, string, error) {
	d, err := os.MkdirTemp("", "test-uds-")
	if err != nil {
		return "", "", err
	}

	return filepath.Join(d, "api.sock"), d, nil
}

// NewUnixAddr creates a temporary socket path and registers directory cleanup.
func NewUnixAddr(t testing.TB) string {
	t.Helper()

	path, dir, err := CreateUnixAddr()
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return path
}

// GetFreeTCPAddress finds an available port on the loopback interface.
func GetFreeTCPAddress() (string, error) {
	lc := net.ListenConfig{}

	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer lis.Close()

	return lis.Addr().String(), nil
}

// MustGetFreeTCPAddress finds an available port or fails the test.
func MustGetFreeTCPAddress(t *testing.T) string {
	t.Helper()

	addr, err := GetFreeTCPAddress()
	if err != nil {
		t.Fatalf("failed to find a free port: %v", err)
	}

	return addr
}

type HealthCondition func(resp *healthpb.HealthCheckResponse) bool

var (
	IsServing = func(r *healthpb.HealthCheckResponse) bool {
		return r.Status == healthpb.HealthCheckResponse_SERVING
	}

	IsNotServing = func(r *healthpb.HealthCheckResponse) bool {
		return r.Status == healthpb.HealthCheckResponse_NOT_SERVING
	}
)

// PollHealthStatus polls the health service until the condition is met or the timeout is reached.
func PollHealthStatus(addr string, serviceName string, timeout time.Duration, condition HealthCondition) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Bypass gRPC's internal DNS resolver for IP:Port strings
	dialTarget := fmt.Sprintf("passthrough:///%s", addr)

	conn, err := grpc.NewClient(dialTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create client for %s: %w", addr, err)
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)

	return wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		callCtx, callCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer callCancel()

		resp, err := client.Check(callCtx, &healthpb.HealthCheckRequest{Service: serviceName})
		if err != nil {
			return false, nil
		}

		return condition(resp), nil
	})
}

// WaitForStatus waits for a health condition to be met or fails the test.
func WaitForStatus(t *testing.T, addr string, serviceName string, timeout time.Duration, condition HealthCondition) {
	t.Helper()

	if err := PollHealthStatus(addr, serviceName, timeout, condition); err != nil {
		t.Fatalf("Condition for %s not met on %s within %v: %v", serviceName, addr, timeout, err)
	}
}
