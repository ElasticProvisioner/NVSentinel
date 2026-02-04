# NVIDIA Device API: Watch Example

This example demonstrates how to implement event-driven monitoring using the NVIDIA Device API.

## Concepts

* **Stream Processing**: Implementing a `Watch()` loop that processes a continuous stream of resource events (`ADDED`, `MODIFIED`, `DELETED`).
* **Connection Middleware**: Using gRPC interceptors to inject structured logging and telemetry into the transport layer.
* **Signal Handling**: Managing graceful shutdowns by intercepting system signals (`SIGINT`, `SIGTERM`) to cleanly close active gRPC streams.

---

## Running the Example

1. The watch stream requires an active server to produce events. In a separate terminal, start the provided fake server: `go run ../fake-server/main.go`
2. In this directory, execute the client logic:

```bash
# Default target: unix:///var/run/nvidia-device-api/device-api.sock
go run main.go
```
To stop the application, press `Ctrl+C`

> [!TIP]
> **Permissions**: If you do not have write access to `/var/run/`, run both the server and client with a custom target: `export NVIDIA_DEVICE_API_TARGET="unix:///tmp/device-api.sock"`

### Expected Output

When the example starts, it performs an initial `List` to establish a baseline of the hardware state, followed by a long-lived `Watch` stream. As the server generates synthetic events, the client will log:
1. The total number of GPUs currently known to the node-local server.
2. Confirmation that the gRPC watch stream has been established.
3. The nature of changes, logged as `event` (e.g., `ADDED`, `MODIFIED`).
4. The device `name`, `uuid`, and an evaluated `Ready` status.

---
