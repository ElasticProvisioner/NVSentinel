# NVIDIA Device API: Basic Client Example

This example demonstrates how to perform point-in-time discovery of node-local resources using the NVIDIA Device API Go client.

## Concepts

* **gRPC Transport**: Initializing a client over a Unix Domain Socket (UDS) using the `nvgrpc` package.
* **Kubernetes-Native Interfaces**: Utilizing the generated `clientset` to perform standard `List` and `Get` operations.
* **Condition Evaluation**: Using the `k8s.io/apimachinery/pkg/api/meta` library to interpret resource status conditions.

---

## Running the Example

1. The client requires a running Device API server. In a separate terminal, start the provided fake server: `go run ../fake-server/main.go`
2. In this directory, execute the client logic:

```bash
# Default target: unix:///var/run/nvidia-device-api/device-api.sock
go run main.go
```

> [!TIP]
> **Permissions**: If you do not have write access to `/var/run/`, run both the server and client with a custom target: `export NVIDIA_DEVICE_API_TARGET="unix:///tmp/device-api.sock"`

### Expected Output

When the client successfully connects to the server, it will log the following information:
1. A summary of the total number of GPUs discovered on the node.
2. The name and UUID for each individual device.
3. The current `Ready` status derived from the resource conditions.

> [!TIP]
> If the client cannot connect, verify that the `NVIDIA_DEVICE_API_TARGET` matches the address used by your fake server.

---
