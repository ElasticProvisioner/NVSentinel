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
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	devicev1alpha1 "github.com/nvidia/nvsentinel/api/device/v1alpha1"
	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app"
	"github.com/nvidia/nvsentinel/cmd/device-apiserver/app/options"
	"github.com/nvidia/nvsentinel/pkg/client-go/client/versioned"
	"github.com/nvidia/nvsentinel/pkg/grpc/client"
	"github.com/nvidia/nvsentinel/pkg/util/testutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir := t.TempDir()

	socketPath := testutils.NewUnixAddr(t)
	kineSocket := fmt.Sprintf("unix://%s", testutils.NewUnixAddr(t))
	healthAddr := testutils.GetFreeTCPAddress(t)

	opts := options.NewServerRunOptions()
	opts.NodeName = "test-node"
	opts.GRPC.BindAddress = "unix://" + socketPath
	opts.HealthAddress = healthAddr
	opts.Storage.DatabaseDir = tmpDir
	opts.Storage.DatabasePath = tmpDir + "state.db"
	opts.Storage.KineSocketPath = kineSocket
	opts.Storage.KineConfig.Endpoint = fmt.Sprintf("sqlite://%s/db.sqlite", tmpDir)
	opts.Storage.KineConfig.Listener = kineSocket

	completed, err := opts.Complete(ctx)
	if err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	go func() {
		if err := app.Run(ctx, completed); err != nil && err != context.Canceled {
			t.Errorf("Server exited with error: %v", err)
		}
	}()

	testutils.WaitForStatus(t, healthAddr, "", 5*time.Second, testutils.IsServing)

	config := &client.Config{Target: "unix://" + socketPath}
	client, err := versioned.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to create clientset: %v", err)
	}

	var created *devicev1alpha1.GPU

	t.Run("Create", func(t *testing.T) {
		gpu := &devicev1alpha1.GPU{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-ad2367dd-a40e-6b86-6fc3-c44a2cc92c7e",
			},
			Spec: devicev1alpha1.GPUSpec{
				UUID: "GPU-ad2367dd-a40e-6b86-6fc3-c44a2cc92c7e",
			},
			Status: devicev1alpha1.GPUStatus{
				Conditions: []metav1.Condition{
					{
						Type:    "Ready",
						Status:  metav1.ConditionFalse,
						Reason:  "DriverNotReaady",
						Message: "Driver is posting ready status",
					},
				},
			},
		}

		created, err = client.DeviceV1alpha1().GPUs().Create(ctx, gpu, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create GPU: %v", err)
		}

		// Client generated fields
		if created.Kind != "GPU" {
			t.Errorf("expected kind 'GPU', got %s", created.Kind)
		}
		if created.APIVersion != devicev1alpha1.SchemeGroupVersion.String() {
			t.Errorf("expected version %s, got %s", devicev1alpha1.SchemeGroupVersion.String(), created.APIVersion)
		}

		// Server generated fields
		if created.Namespace != "default" {
			t.Error("Server failed to set default namespace")
		}
		if created.UID == "" {
			t.Error("Server failed to generate a UID for the GPU")
		}
		if created.ResourceVersion == "" {
			t.Error("Server failed to generate a ResourceVersion")
		}
		if created.Generation != 1 {
			t.Error("Server failed to set initial Generation")
		}
		if created.CreationTimestamp.IsZero() {
			t.Error("Server failed to set a CreationTimestamp")
		}

		// Data integrity
		if created.Name != gpu.Name {
			t.Errorf("expected name %q, got %q", gpu.Name, created.Name)
		}
		if created.Spec.UUID != gpu.Spec.UUID {
			t.Errorf("expected UUID %q, got %q", gpu.Spec.UUID, created.Spec.UUID)
		}

		// Data integrity: Status
		if len(created.Status.Conditions) != len(gpu.Status.Conditions) {
			t.Fatalf("expected %d conditions, got %d", len(gpu.Status.Conditions), len(created.Status.Conditions))
		}

		cond := created.Status.Conditions[0]
		expected := gpu.Status.Conditions[0]

		if cond.Type != expected.Type {
			t.Errorf("expected condition Type %q, got %q", expected.Type, cond.Type)
		}
		if cond.Status != expected.Status {
			t.Errorf("expected condition Status %q, got %q", expected.Status, cond.Status)
		}
		if cond.Reason != expected.Reason {
			t.Errorf("expected condition Reason %q, got %q", expected.Reason, cond.Reason)
		}
		if cond.Message != expected.Message {
			t.Errorf("expected condition Message %q, got %q", expected.Message, cond.Message)
		}
		if cond.LastTransitionTime.IsZero() {
			t.Error("condition LastTransitionTime should not be zero")
		}

		// TODO: remove
		objJson, _ := json.MarshalIndent(created, "", "  ")
		fmt.Printf("\n--- [Object After Creation] ---\n%s\n", string(objJson))
	})

	t.Run("Update", func(t *testing.T) {
		if created == nil {
			t.Skip("Skipping: Create failed")
		}

		toUpdate := created.DeepCopy()
		toUpdate.Spec.UUID = "GPU-cd2367dd-a40e-6b86-6fc3-c44a2cc92c7d"

		updated, err := client.DeviceV1alpha1().GPUs().Update(ctx, toUpdate, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Failed to update GPU: %v", err)
		}

		if updated.Spec.UUID != toUpdate.Spec.UUID {
			t.Errorf("expected UUID %q, got %q", toUpdate.Spec.UUID, updated.Spec.UUID)
		}

		oldRV, _ := strconv.ParseInt(created.ResourceVersion, 10, 64)
		updatedRV, _ := strconv.ParseInt(updated.ResourceVersion, 10, 64)

		if updatedRV <= oldRV {
			t.Errorf("expected ResourceVersion to increase, got %d (old) and %d (new)", oldRV, updatedRV)
		}

		if updated.Generation <= created.Generation {
			t.Errorf("expected Generation to increase, got %d (old) and %d (new)", created.Generation, updated.Generation)
		}

		// TODO: remove
		objJson, _ := json.MarshalIndent(updated, "", "  ")
		fmt.Printf("\n--- [Object After Update] ---\n%s\n", string(objJson))
	})

	// TODO: add tests for Delete, List, Watch
}
