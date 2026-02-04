# NVIDIA Device API: Controller Example

This example demonstrates how to integrate the NVIDIA Device API with `controller-runtime`.

## Concepts

- **Informer Injection**: The `Manager` is configured with a custom `NewInformer` function that injects a gRPC-backed `SharedIndexInformer`. This bypasses the standard Kubernetes API server for `GPU` resources.
- **Root-Scoped Resources**: Since the Device API does not currently provide a discovery endpoint, a custom `RESTMapper` is used to define GPU resources as root-scoped (non-namespaced).
- **Hybrid Connectivity**: The manager remains capable of "hybrid" operations—using gRPC for hardware resources while falling back to standard Kubernetes transport for core types (e.g., Pods or Events).
- **Transparent Caching**: Standard `mgr.GetClient()` calls within the `Reconcile` loop are automatically and transparently routed to the high-performance local gRPC cache.
- **Controller-Runtime Standard Patterns**: Using the standard `builder` pattern to react to hardware events exactly like standard Kubernetes objects.

---

## Running the Example

1. The controller requires a data source to populate its cache. In a separate terminal, start the provided fake server: `go run ../fake-server/main.go`
2. In this directory, execute the controller logic:

```bash
# Default target: unix:///var/run/nvidia-device-api/device-api.sock
go run main.go
```
To stop the controller, press `Ctrl+C`

> [!TIP]
> **Permissions**: If you do not have write access to `/var/run/`, run both the server and client with a custom target: `export NVIDIA_DEVICE_API_TARGET="unix:///tmp/device-api.sock"`

### Expected Output

Upon startup, the manager initializes the `controller-runtime` stack and synchronizes the local cache with the Device API server. You will observe the following sequence:
1. Logs indicating the metrics server, informer caches, and controller workers are starting.
2. The controller will list all existing GPUs from the gRPC endpoint to "warm" the cache.
3. As the Fake Server generates events, the controller's `Reconcile` loop is triggered, logs identifying the `name` and `uuid` of the GPU currently being processed.

---
