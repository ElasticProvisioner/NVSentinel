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

package v1alpha1

import (
	"context"
	"testing"

	devicev1alpha1 "github.com/nvidia/nvsentinel/api/device/v1alpha1"
	pb "github.com/nvidia/nvsentinel/internal/generated/device/v1alpha1"
	"github.com/nvidia/nvsentinel/pkg/storage/memory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

func newTestService(t *testing.T) *gpuService {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := devicev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	codecs := serializer.NewCodecFactory(scheme)
	gv := devicev1alpha1.SchemeGroupVersion
	info, _ := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), runtime.ContentTypeJSON)
	codec := codecs.CodecForVersions(info.Serializer, info.Serializer, schema.GroupVersions{gv}, schema.GroupVersions{gv})

	s, destroy, err := memory.CreateStorage(codec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(destroy)

	return NewGPUService(s, destroy)
}

func createTestGpu(t *testing.T, svc *gpuService, name string) *pb.Gpu {
	t.Helper()

	gpu, err := svc.CreateGpu(context.Background(), &pb.CreateGpuRequest{
		Gpu: &pb.Gpu{
			Metadata: &pb.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: &pb.GpuSpec{
				Uuid: "GPU-" + name,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create GPU %q: %v", name, err)
	}

	return gpu
}

func TestGPUService_CreateAndGet(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created := createTestGpu(t, svc, "gpu-0")

	if created.GetMetadata().GetName() != "gpu-0" {
		t.Errorf("expected name %q, got %q", "gpu-0", created.GetMetadata().GetName())
	}
	if created.GetMetadata().GetUid() == "" {
		t.Error("expected UID to be set on created GPU")
	}

	resp, err := svc.GetGpu(ctx, &pb.GetGpuRequest{
		Name:      "gpu-0",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("GetGpu failed: %v", err)
	}

	got := resp.GetGpu()
	if got.GetMetadata().GetName() != "gpu-0" {
		t.Errorf("expected name %q, got %q", "gpu-0", got.GetMetadata().GetName())
	}
	if got.GetMetadata().GetUid() != created.GetMetadata().GetUid() {
		t.Errorf("UID mismatch: expected %q, got %q",
			created.GetMetadata().GetUid(), got.GetMetadata().GetUid())
	}
}

func TestGPUService_CreateDuplicate(t *testing.T) {
	svc := newTestService(t)

	createTestGpu(t, svc, "gpu-dup")

	_, err := svc.CreateGpu(context.Background(), &pb.CreateGpuRequest{
		Gpu: &pb.Gpu{
			Metadata: &pb.ObjectMeta{
				Name:      "gpu-dup",
				Namespace: "default",
			},
			Spec: &pb.GpuSpec{
				Uuid: "GPU-dup",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for duplicate create, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.AlreadyExists {
		t.Errorf("expected code %v, got %v: %s", codes.AlreadyExists, st.Code(), st.Message())
	}
}

func TestGPUService_List(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestGpu(t, svc, "gpu-a")
	createTestGpu(t, svc, "gpu-b")

	resp, err := svc.ListGpus(ctx, &pb.ListGpusRequest{
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("ListGpus failed: %v", err)
	}

	count := len(resp.GetGpuList().GetItems())
	if count != 2 {
		t.Errorf("expected 2 GPUs, got %d", count)
	}
}

func TestGPUService_Delete(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestGpu(t, svc, "gpu-del")

	_, err := svc.DeleteGpu(ctx, &pb.DeleteGpuRequest{
		Name:      "gpu-del",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("DeleteGpu failed: %v", err)
	}

	_, err = svc.GetGpu(ctx, &pb.GetGpuRequest{
		Name:      "gpu-del",
		Namespace: "default",
	})
	if err == nil {
		t.Fatal("expected NotFound after delete, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected code %v, got %v: %s", codes.NotFound, st.Code(), st.Message())
	}
}

func TestGPUService_DeleteNotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.DeleteGpu(context.Background(), &pb.DeleteGpuRequest{
		Name:      "nonexistent",
		Namespace: "default",
	})
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected code %v, got %v: %s", codes.NotFound, st.Code(), st.Message())
	}
}

func TestGPUService_Update(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created := createTestGpu(t, svc, "gpu-upd")

	updated, err := svc.UpdateGpu(ctx, &pb.UpdateGpuRequest{
		Gpu: &pb.Gpu{
			Metadata: &pb.ObjectMeta{
				Name:      "gpu-upd",
				Namespace: "default",
			},
			Spec: &pb.GpuSpec{
				Uuid: "GPU-new-uuid",
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpu failed: %v", err)
	}

	if updated.GetSpec().GetUuid() != "GPU-new-uuid" {
		t.Errorf("expected spec.uuid %q, got %q", "GPU-new-uuid", updated.GetSpec().GetUuid())
	}
	if updated.GetMetadata().GetGeneration() != created.GetMetadata().GetGeneration()+1 {
		t.Errorf("expected generation %d, got %d",
			created.GetMetadata().GetGeneration()+1, updated.GetMetadata().GetGeneration())
	}
}

func TestGPUService_UpdateStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created := createTestGpu(t, svc, "gpu-status")

	updated, err := svc.UpdateGpuStatus(ctx, &pb.UpdateGpuStatusRequest{
		Name: "gpu-status",
		Status: &pb.GpuStatus{
			RecommendedAction: "drain",
		},
	})
	if err != nil {
		t.Fatalf("UpdateGpuStatus failed: %v", err)
	}

	if updated.GetStatus().GetRecommendedAction() != "drain" {
		t.Errorf("expected recommended action %q, got %q",
			"drain", updated.GetStatus().GetRecommendedAction())
	}

	// Generation must NOT change on status-only updates.
	if updated.GetMetadata().GetGeneration() != created.GetMetadata().GetGeneration() {
		t.Errorf("expected generation %d (unchanged), got %d",
			created.GetMetadata().GetGeneration(), updated.GetMetadata().GetGeneration())
	}
}

func TestGPUService_CreateValidation(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		name string
		req  *pb.CreateGpuRequest
	}{
		{
			name: "nil gpu body",
			req:  &pb.CreateGpuRequest{},
		},
		{
			name: "nil metadata",
			req: &pb.CreateGpuRequest{
				Gpu: &pb.Gpu{
					Spec: &pb.GpuSpec{Uuid: "GPU-test"},
				},
			},
		},
		{
			name: "empty name",
			req: &pb.CreateGpuRequest{
				Gpu: &pb.Gpu{
					Metadata: &pb.ObjectMeta{Name: ""},
					Spec:     &pb.GpuSpec{Uuid: "GPU-test"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateGpu(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected InvalidArgument error, got nil")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %T: %v", err, err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected code %v, got %v: %s", codes.InvalidArgument, st.Code(), st.Message())
			}
		})
	}
}
