# Configuration

The code generation pipeline relies on specific versions of upstream tools.

The source of truth for these versions is the root [/.versions.yaml](../../.versions.yaml) file.

## Version Precedence

Versions are resolved in the following order:
1.  **Override**: Environment variables set in the shell.
2.  **Default**: The values defined in `.versions.yaml`.
3.  **Fallback**: The versions specified in the `go.mod` file of the generator.

## Overriding Versions

You can override defaults by setting environment variables before sourcing `kube_codegen.sh` or running `hack/update-codegen.sh`.

| Environment Variable      | Tool Category                | Key in `.versions.yaml` |
|---------------------------|------------------------------|-------------------------|
| `KUBE_CODEGEN_TAG`        | Kubernetes Generators        | `kubernetes_code_gen`   |
| `PROTOC_GEN_GO_TAG`       | Protobuf Go Plugin           | `protoc_gen_go`         |
| `PROTOC_GEN_GO_GRPC_TAG`  | gRPC Go Plugin               | `protoc_gen_go_grpc`    |
| `GOVERTER_TAG`            | Proto-to-Go Mapper           | `goverter`              |

### Example: Testing a New Kubernetes Version

To test the generator against a different version of the Kubernetes API without updating the entire repository:

```bash
export KUBE_CODEGEN_TAG="v0.32.0"
./hack/update-codegen.sh
```
