# API Development

## Workflow

Follow these steps to add new resources or update existing fields:
1. **Go Definitions**: Update `device/${VERSION}/${TYPE}_types.go`. Ensure you include the necessary marker comments (e.g., `// +k8s:deepcopy-gen`) required by the generators.
2. **Proto Definitions**: Update `proto/device/${VERSION}/${TYPE}.proto`. Ensure Protobuf field numbers are never reused or changed once released.
3. **Registration**: If adding a new **Kind**, register it in `device/${VERSION}/register.go` within the `addKnownTypes` function.
4. **Conversion Logic**: Update the mapping interface in `device/${VERSION}/converter.go`. See [Goverter](https://github.com/jmattheis/goverter) documentation for additional details.
5. **Generate**: Run `make code-gen`. This orchestrates `protoc`, `deepcopy-gen`, and `goverter` to refresh all artifacts.

## Conventions

- **Kubernetes Resource Model (KRM)**:
  - Go type definitions must strictly follow the standard [Kubernetes Resource Model](https://github.com/kubernetes/design-proposals-archive/blob/main/architecture/resource-management.md).
  - **Separation of Concerns**: Use `Spec` for desired configuration and `Status` for observed state.

## Housekeeping

If your generated files are out of sync or contain stale data:

```bash
# Removes generated code (bindings, deepcopy, goverter)
make clean
```
