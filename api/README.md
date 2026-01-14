# API Definitions

This module contains the canonical API definitions for the **NVIDIA Device API**. It serves as the single source of truth for resource schemas, gRPC wire formats, and Kubernetes-native Go types.

## Structure

* **`device/`**: Contains the **Kubernetes Resource Model (KRM)** definitions. These types implement the `runtime.Object` interface.
* **`proto/`**: Contains the **Language-Agnostic Definitions**. Protobuf messages and gRPC service definitions that define the node-local communication contract.
* **`gen/go/`**: Contains the **Bindings**. The output of the `protoc` compiler (`.pb.go` and `_grpc.pb.go`).

## Code Generation

This module relies on three distinct generation phases to maintain type safety:
1. **Protobuf**: Compiles `.proto` files into Go structs.
2. **DeepCopy**: Generates `zz_generated.deepcopy.go` to support Kubernetes object manipulation.
3. **Conversion**: Generates `zz_generated.goverter.go` to map between Protobuf messages and KRM Go types (using Goverter).

To run the full pipeline:

```bash
make code-gen
```

## Development

Refer to [DEVELOPMENT.md](DEVELOPMENT.md) for instructions on adding new fields or resources to the API.
