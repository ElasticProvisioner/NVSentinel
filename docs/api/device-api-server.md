# Device API Server - API Reference

This document provides the complete API reference for the Device API Server gRPC services.

## Overview

The Device API Server exposes two gRPC services:

| Service | Purpose | Clients |
|---------|---------|---------|
| `GpuService` | Read GPU states, watch for changes | Consumers (device plugins, DRA drivers) |
| `ProviderService` | Register GPUs, update health status | Providers (health monitors, NVML) |

**Package**: `nvidia.device.v1alpha1`

**Connection Endpoints**:
- Unix Socket: `unix:///var/run/device-api/device.sock` (recommended)
- TCP: `localhost:50051`

## GpuService

The `GpuService` provides read-only access to GPU resources for consumers.

### GetGpu

Retrieves a single GPU resource by its unique name.

```protobuf
rpc GetGpu(GetGpuRequest) returns (GetGpuResponse);
```

**Request**:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | The unique resource name of the GPU |

**Response**:

| Field | Type | Description |
|-------|------|-------------|
| `gpu` | Gpu | The requested GPU resource |

**Errors**:
- `NOT_FOUND`: GPU with the specified name does not exist

**Example**:

```bash
grpcurl -plaintext localhost:50051 \
  -d '{"name": "gpu-abc123"}' \
  nvidia.device.v1alpha1.GpuService/GetGpu
```

### ListGpus

Retrieves a list of all GPU resources.

```protobuf
rpc ListGpus(ListGpusRequest) returns (ListGpusResponse);
```

**Request**: Empty (reserved for future filtering/pagination)

**Response**:

| Field | Type | Description |
|-------|------|-------------|
| `gpu_list` | GpuList | List of all GPU resources |

**Example**:

```bash
grpcurl -plaintext localhost:50051 \
  nvidia.device.v1alpha1.GpuService/ListGpus
```

**Response Example**:

```json
{
  "gpuList": {
    "items": [
      {
        "name": "gpu-abc123",
        "spec": {
          "uuid": "GPU-a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6"
        },
        "status": {
          "conditions": [
            {
              "type": "Ready",
              "status": "True",
              "lastTransitionTime": "2026-01-21T10:00:00Z",
              "reason": "GPUHealthy",
              "message": "GPU is healthy and available"
            }
          ]
        },
        "resourceVersion": "42"
      }
    ]
  }
}
```

### WatchGpus

Streams lifecycle events for GPU resources. The stream remains open until the client disconnects or an error occurs.

```protobuf
rpc WatchGpus(WatchGpusRequest) returns (stream WatchGpusResponse);
```

**Request**: Empty (reserved for future filtering/resumption)

**Response Stream**:

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Event type: `ADDED`, `MODIFIED`, `DELETED`, `ERROR` |
| `object` | Gpu | The GPU resource (last known state for DELETED) |

**Event Types**:

| Type | Description |
|------|-------------|
| `ADDED` | GPU was registered or first observed |
| `MODIFIED` | GPU status was updated |
| `DELETED` | GPU was unregistered |
| `ERROR` | An error occurred in the watch stream |

**Example**:

```bash
grpcurl -plaintext localhost:50051 \
  nvidia.device.v1alpha1.GpuService/WatchGpus
```

**Behavior**:
- On connection, receives `ADDED` events for all existing GPUs
- Subsequent events reflect real-time changes
- Stream is per-client; multiple clients can watch simultaneously

---

## ProviderService

The `ProviderService` allows health monitors (providers) to register and update GPU device states.

> **Important**: Write operations acquire exclusive locks, blocking all consumer reads until completion. This prevents consumers from reading stale "healthy" states during GPU health transitions.

### RegisterGpu

Registers a new GPU with the server.

```protobuf
rpc RegisterGpu(RegisterGpuRequest) returns (RegisterGpuResponse);
```

**Request**:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique logical identifier (e.g., `gpu-abc123`) |
| `spec` | GpuSpec | GPU identity (UUID) |
| `provider_id` | string | Optional provider identifier |
| `initial_status` | GpuStatus | Optional initial status |

**Response**:

| Field | Type | Description |
|-------|------|-------------|
| `created` | bool | True if new GPU was created, false if already existed |
| `resource_version` | int64 | Current version after registration |

**Behavior**:
- If GPU already exists, returns existing GPU without modification
- Triggers `ADDED` event for active watch streams

**Example**:

```bash
grpcurl -plaintext localhost:50051 \
  -d '{
    "name": "gpu-abc123",
    "spec": {"uuid": "GPU-a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6"},
    "provider_id": "nvml-provider"
  }' \
  nvidia.device.v1alpha1.ProviderService/RegisterGpu
```

### UnregisterGpu

Removes a GPU from the server.

```protobuf
rpc UnregisterGpu(UnregisterGpuRequest) returns (UnregisterGpuResponse);
```

**Request**:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique identifier of GPU to remove |

**Response**:

| Field | Type | Description |
|-------|------|-------------|
| `deleted` | bool | True if GPU was found and deleted |

**Behavior**:
- GPU will no longer appear in ListGpus/GetGpu responses
- Triggers `DELETED` event for active watch streams

### UpdateGpuStatus

Replaces the entire status of a registered GPU.

```protobuf
rpc UpdateGpuStatus(UpdateGpuStatusRequest) returns (UpdateGpuStatusResponse);
```

**Request**:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique identifier of GPU to update |
| `status` | GpuStatus | New status (completely replaces existing) |
| `provider_id` | string | Optional provider identifier |

**Response**:

| Field | Type | Description |
|-------|------|-------------|
| `resource_version` | int64 | New version after update |

**Errors**:
- `NOT_FOUND`: GPU is not registered

**Locking**: Acquires exclusive write lock, blocking all reads.

### UpdateGpuCondition

Updates or adds a single condition on a GPU without affecting other conditions.

```protobuf
rpc UpdateGpuCondition(UpdateGpuConditionRequest) returns (UpdateGpuConditionResponse);
```

**Request**:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique identifier of GPU to update |
| `condition` | Condition | Condition to set/update |
| `provider_id` | string | Optional provider identifier |

**Response**:

| Field | Type | Description |
|-------|------|-------------|
| `resource_version` | int64 | New version after update |

**Behavior**:
- If condition with same `type` exists, it is replaced
- If no condition with that `type` exists, it is added
- Other conditions remain unchanged

**Example** (mark GPU unhealthy due to XID error):

```bash
grpcurl -plaintext localhost:50051 \
  -d '{
    "name": "gpu-abc123",
    "condition": {
      "type": "Ready",
      "status": "False",
      "reason": "XidError",
      "message": "Critical XID error 79 detected"
    }
  }' \
  nvidia.device.v1alpha1.ProviderService/UpdateGpuCondition
```

### Heartbeat

Allows providers to signal liveness. (Future enhancement)

```protobuf
rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
```

---

## Resource Types

### Gpu

The main GPU resource following the Kubernetes Resource Model pattern.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique logical identifier |
| `spec` | GpuSpec | Identity and desired attributes |
| `status` | GpuStatus | Most recently observed state |
| `resource_version` | int64 | Monotonically increasing version |

### GpuSpec

Defines the identity of a GPU.

| Field | Type | Description |
|-------|------|-------------|
| `uuid` | string | Physical hardware UUID (e.g., `GPU-a1b2c3d4-...`) |

### GpuStatus

Contains the observed state of a GPU.

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | Condition[] | Current state observations |
| `recommended_action` | string | Suggested resolution for negative states |

### Condition

Describes one aspect of the GPU's current state.

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Category (e.g., `Ready`, `MemoryHealthy`) |
| `status` | string | `True`, `False`, or `Unknown` |
| `last_transition_time` | Timestamp | When status last changed |
| `reason` | string | Machine-readable reason (UpperCamelCase) |
| `message` | string | Human-readable details |

**Standard Condition Types**:

| Type | Description |
|------|-------------|
| `Ready` | Overall GPU health and availability |
| `MemoryHealthy` | GPU memory is functioning correctly |
| `ThermalHealthy` | GPU temperature is within safe limits |

---

## Go Client Example

```go
package main

import (
    "context"
    "log"

    v1alpha1 "github.com/nvidia/device-api/api/gen/go/device/v1alpha1"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    // Connect via Unix socket (recommended)
    conn, err := grpc.NewClient(
        "unix:///var/run/device-api/device.sock",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        log.Fatalf("failed to connect: %v", err)
    }
    defer conn.Close()

    // Consumer: List GPUs
    gpuClient := v1alpha1.NewGpuServiceClient(conn)
    resp, err := gpuClient.ListGpus(context.Background(), &v1alpha1.ListGpusRequest{})
    if err != nil {
        log.Fatalf("failed to list GPUs: %v", err)
    }

    for _, gpu := range resp.GpuList.Items {
        log.Printf("GPU: %s, Version: %d", gpu.Name, gpu.ResourceVersion)
        for _, cond := range gpu.Status.Conditions {
            log.Printf("  Condition: %s=%s (%s)", cond.Type, cond.Status, cond.Reason)
        }
    }

    // Provider: Update GPU condition
    providerClient := v1alpha1.NewProviderServiceClient(conn)
    _, err = providerClient.UpdateGpuCondition(context.Background(),
        &v1alpha1.UpdateGpuConditionRequest{
            Name: "gpu-abc123",
            Condition: &v1alpha1.Condition{
                Type:    "Ready",
                Status:  "False",
                Reason:  "XidError",
                Message: "Critical XID 79 detected",
            },
        })
    if err != nil {
        log.Fatalf("failed to update condition: %v", err)
    }
}
```

---

## Error Codes

| Code | Meaning |
|------|---------|
| `NOT_FOUND` | GPU with specified name does not exist |
| `INVALID_ARGUMENT` | Request contains invalid parameters |
| `INTERNAL` | Server-side error occurred |
| `UNAVAILABLE` | Server is temporarily unavailable |

---

## See Also

- [Operations Guide](../operations/device-api-server.md)
- [Design Document](../design/device-api-server.md)
- [NVML Fallback Provider](../design/nvml-fallback-provider.md)
