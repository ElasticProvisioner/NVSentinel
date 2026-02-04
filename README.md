# NVIDIA Device API

**The NVIDIA Device API allows you to query and manipulate the state of node-local resources (such as GPUs) in Kubernetes**. Unlike the cluster-wide Kubernetes API, the Device API operates exclusively at the node level.

The core control plane is the Device API server and the gRPC API that it exposes. Node-level agents, local monitoring tools, and external components communicate with one another through this node-local Device API server rather than the central Kubernetes control plane.

NVIDIA provides a [client library](./pkg/client-go) for those looking to write applications using the Device API. This library allows you to query and manipulate node-local resources using standard Kubernetes interfaces. Alternatively, the API can be accessed directly via gRPC.

---

## Quick Start

```go
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/nvidia/nvsentinel/pkg/client-go/clientset/versioned"
	"github.com/nvidia/nvsentinel/pkg/grpc/client"
)

func main() {
    ctx := context.Background()

    // Connect to the local node's Device API server
    config := &client.Config{Target: "unix:///var/run/nvidia-device-api/device-api.sock"}
    clientset := versioned.NewForConfigOrDie(config)

    // Standard Kubernetes-style List call
    gpus, err := clientset.DeviceV1alpha1().GPUs().List(ctx, metav1.ListOptions{})
    if err != nil {
        panic(err)
    }
}
```

See [examples](./examples) for additional details.

---

## Components

### Device API Server
The `device-apiserver` is a node-local control plane for NVIDIA devices.

**Running the server**:
```bash
# Build the binary
make build

# Start the server with a local database
./bin/device-apiserver \
    --bind-address="unix:///var/run/nvidia-device-api/device-api.sock" \
    --datastore-endpoint="sqlite:///var/lib/nvidia-device-api/state.db"
```

---

## Development

### Prerequisites

* **Go**: `v1.25+`
* **Protoc**: Required for protobuf generation.
* **Make**

### Workflow
The project utilizes a unified generation pipeline. **Avoid editing generated files directly**. If Protobuf definitions (`.proto`) or Go types (`_types.go`) are modified, run the following commands to synchronize the repository:

```bash
# Sync all gRPC bindings, DeepCopy/Conversion methods, Clients, and Server
make code-gen

# Run tests
make test

# Verify code quality
make lint

# Optional: Run integration tests
make test-integration
```

---

## Contributing

We welcome contributions! Please see:

- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Development Guide](DEVELOPMENT.md)

All contributors must sign their commits (DCO).

--- 

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

---

*Built by NVIDIA for GPU infrastructure management*
