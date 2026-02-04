# NVIDIA Device API: Go Client Examples

This directory contains examples demonstrating various use cases and functionality of the NVIDIA Device API Go client.

---

## Integration Patterns

| Example | Focus | Use Case |
| :--- | :--- | :--- |
| **[Basic Client](./client)** | **Point-in-Time Discovery** | CLI tools and scripts |
| **[Watch Monitor](./watch)** | **Asynchronous Operations** | Real-time event-driven monitoring |
| **[Controller](./controller)** | **State Enforcement** | `controller-runtime` Reconcilers driven by node-local state |
| **[Fake Client](./fake-client)** | **Unit Testing** | Testing logic using the in-memory `ObjectTracker` without a server |

---

## Prerequisites

The examples utilize a [Fake Device API Server](./fake-server) to simulate a local environment. This server maintains an in-memory inventory of mock GPUs and generates synthetic status events (e.g., toggling `Ready` conditions).

```bash
# Start the simulated device apiserver
sudo go run ./fake-server/main.go
```
To stop the server, press `Ctrl+C`

## Running Examples

With the server running, navigate to an example directory and run it:

```bash
# Example: Running the basic client
cd client/
sudo go run main.go
```

> [!TIP] 
> **Permissions**: By default, the server attempts to create a socket in `var/run`. If you do not have root privileges, override the target path using an environment variable: `export NVIDIA_DEVICE_API_TARGET="unix:///tmp/device-api.sock"`

---
