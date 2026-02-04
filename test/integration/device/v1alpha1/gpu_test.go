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
	"reflect"
	"strconv"
	"testing"
	"time"

	devicev1alpha1 "github.com/nvidia/nvsentinel/api/device/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGPU(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := clientset

	gpu1 := &devicev1alpha1.GPU{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-11111111-1111-1111-1111-111111111111",
		},
		Spec: devicev1alpha1.GPUSpec{
			UUID: "GPU-11111111-1111-1111-1111-111111111111",
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
	gpu2 := &devicev1alpha1.GPU{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-22222222-2222-2222-2222-222222222222",
		},
		Spec: devicev1alpha1.GPUSpec{
			UUID: "GPU-22222222-2222-2222-2222-222222222222",
		},
		Status: devicev1alpha1.GPUStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "DriverReaady",
					Message: "Driver is posting ready status",
				},
			},
		},
	}
	gpu3 := &devicev1alpha1.GPU{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-33333333-3333-3333-3333-333333333333",
			Namespace: "default",
		},
		Spec: devicev1alpha1.GPUSpec{
			UUID: "GPU-33333333-3333-3333-3333-333333333333",
		},
		Status: devicev1alpha1.GPUStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "DriverReaady",
					Message: "Driver is posting ready status",
				},
			},
		},
	}

	var created1, created2, created3 *devicev1alpha1.GPU
	var err error

	t.Run("Create", func(t *testing.T) {
		created1, err = client.DeviceV1alpha1().GPUs().Create(ctx, gpu1, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create GPU: %v", err)
		}

		// Client generated fields
		if created1.Kind != "GPU" {
			t.Errorf("expected kind 'GPU', got %s", created1.Kind)
		}
		if created1.APIVersion != devicev1alpha1.SchemeGroupVersion.String() {
			t.Errorf("expected version %s, got %s", devicev1alpha1.SchemeGroupVersion.String(), created1.APIVersion)
		}

		// Server generated fields
		if created1.Namespace != "default" {
			t.Error("server failed to set default namespace")
		}
		if created1.UID == "" {
			t.Error("server failed to generate a UID for the GPU")
		}
		if created1.ResourceVersion == "" {
			t.Error("server failed to generate a ResourceVersion")
		}
		if created1.Generation != 1 {
			t.Error("server failed to set initial Generation")
		}
		if created1.CreationTimestamp.IsZero() {
			t.Error("server failed to set a CreationTimestamp")
		}

		// Data integrity
		if created1.Name != gpu1.Name {
			t.Errorf("expected name %q, got %q", gpu1.Name, created1.Name)
		}
		if created1.Spec.UUID != gpu1.Spec.UUID {
			t.Errorf("expected UUID %q, got %q", gpu1.Spec.UUID, created1.Spec.UUID)
		}

		// Data integrity: Status
		if len(created1.Status.Conditions) != len(gpu1.Status.Conditions) {
			t.Fatalf("Expected %d conditions, got %d", len(gpu1.Status.Conditions), len(created1.Status.Conditions))
		}

		cond := created1.Status.Conditions[0]
		expected := gpu1.Status.Conditions[0]

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
	})

	t.Run("Update", func(t *testing.T) {
		toUpdate := created1.DeepCopy()
		toUpdate.Spec.UUID = "GPU-updated1-1111-1111-1111-111111111111"

		updated, err := client.DeviceV1alpha1().GPUs().Update(ctx, toUpdate, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Failed to update GPU: %v", err)
		}

		// Metadata
		oldRV, _ := strconv.ParseInt(created1.ResourceVersion, 10, 64)
		updatedRV, _ := strconv.ParseInt(updated.ResourceVersion, 10, 64)

		if updated.UID != created1.UID {
			t.Errorf("expected UID to remain the same, got %v (old) and %v (new)", created1.UID, updated.UID)
		}
		if updated.Namespace != created1.Namespace {
			t.Errorf("expected Namespace to remain the same, got %v (old) and %v (new)", created1.Namespace, updated.Namespace)
		}
		if updated.Name != created1.Name {
			t.Errorf("expected Name to remain the same, got %v (old) and %v (new)", created1.Name, updated.Name)
		}
		if updatedRV <= oldRV {
			t.Errorf("expected ResourceVersion to increase, got %d (old) and %d (new)", oldRV, updatedRV)
		}
		if updated.Generation <= created1.Generation {
			t.Errorf("expected Generation to increase, got %d (old) and %d (new)", created1.Generation, updated.Generation)
		}
		if updated.CreationTimestamp != created1.CreationTimestamp {
			t.Errorf("expected CreationTimestamp to remain the same, got %v (old) and %v (new)", created1.CreationTimestamp, updated.CreationTimestamp)
		}

		// Spec
		if updated.Spec.UUID != toUpdate.Spec.UUID {
			t.Errorf("expected UUID %q, got %q", toUpdate.Spec.UUID, updated.Spec.UUID)
		}

		// Status
		if !reflect.DeepEqual(updated.Status, created1.Status) {
			t.Errorf("Status changed during spec update!\nOld: %+v\nNew: %+v",
				created1.Status, updated.Status)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		t.Skip("Skipping: API not implemented")
	})

	t.Run("Patch", func(t *testing.T) {
		t.Skip("Skipping: API not implemented")
	})

	t.Run("List", func(t *testing.T) {
		created2, err = client.DeviceV1alpha1().GPUs().Create(ctx, gpu2, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create GPU: %v", err)
		}

		list, err := client.DeviceV1alpha1().GPUs().List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list GPUs: %v", err)
		}

		if len(list.Items) != 2 {
			t.Errorf("expected 2 GPUs in list, got %d", len(list.Items))
		}

		expectedNames := map[string]bool{
			created1.Name: false,
			created2.Name: false,
		}
		for _, item := range list.Items {
			if _, exists := expectedNames[item.Name]; exists {
				expectedNames[item.Name] = true
			}
		}
		for name, found := range expectedNames {
			if !found {
				t.Errorf("expected GPU %q was not found in the filtered list", name)
			}
		}

		lastObjectRV, _ := strconv.ParseUint(created2.ResourceVersion, 10, 64)
		listRV, _ := strconv.ParseUint(list.ResourceVersion, 10, 64)
		if listRV < lastObjectRV {
			t.Errorf("ResourceVersion (%d) is behind the last object's RV (%d)", listRV, lastObjectRV)
		}

	})

	t.Run("ListAndWatch", func(t *testing.T) {
		// Start watching from the RV of GPU 2
		opts := metav1.ListOptions{
			ResourceVersion: created2.ResourceVersion,
		}

		watcher, err := client.DeviceV1alpha1().GPUs().Watch(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to start watch: %v", err)
		}
		defer watcher.Stop()

		created3, err = client.DeviceV1alpha1().GPUs().Create(ctx, gpu3, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create third GPU: %v", err)
		}

		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				t.Fatal("Watch channel closed prematurely")
			}

			gpu := event.Object.(*devicev1alpha1.GPU)
			if gpu.Name != created3.Name {
				t.Errorf("expected event for %s, got %s", created3.Name, gpu.Name)
			}
			if event.Type != "ADDED" {
				t.Errorf("expected event type ADDED, got %s", event.Type)
			}

		case <-time.After(5 * time.Second):
			t.Fatal("Timed out waiting for Watch event for GPU 3")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.DeviceV1alpha1().GPUs().Delete(ctx, created1.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Failed to delete GPU %q: %v", created1.Name, err)
		}

		_, err = client.DeviceV1alpha1().GPUs().Get(ctx, created1.Name, metav1.GetOptions{})
		if err == nil {
			t.Errorf("expected error when getting deleted GPU %q, but got nil", created1.Name)
		}

		err = client.DeviceV1alpha1().GPUs().Delete(ctx, created1.Name, metav1.DeleteOptions{})
		if err == nil {
			t.Error("expected error when deleting already-deleted GPU, but got nil")
		}

		list, err := client.DeviceV1alpha1().GPUs().List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list GPUs after delete: %v", err)
		}

		expectedCount := 2
		if len(list.Items) != expectedCount {
			t.Errorf("expected %d GPUs remaining, got %d", expectedCount, len(list.Items))
		}

		for _, item := range list.Items {
			if item.Name == created1.Name {
				t.Errorf("deleted GPU %q still present in list", created1.Name)
			}
		}

		lastObjectRV, _ := strconv.ParseUint(created3.ResourceVersion, 10, 64)
		listRV, _ := strconv.ParseUint(list.ResourceVersion, 10, 64)
		if listRV < lastObjectRV {
			t.Errorf("ResourceVersion (%d) is behind the last object's RV (%d)", listRV, lastObjectRV)
		}
	})
}
