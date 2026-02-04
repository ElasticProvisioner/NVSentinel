# Development Guide

---

## Code Generation

This project relies heavily on generated code to ensure consistency with the Kubernetes API machinery.

### Generation Pipeline
The `make code-gen` command orchestrates several tools:

1. **Protoc**: Generates gRPC Go bindings from `api/proto`.
2. **Goverter**: Generates type-safe conversion logic between internal gRPC types and the Kubernetes-style API types defined in `api/device/`.
3. **K8s Code-Gen**:
  - Generates `DeepCopy` methods for API types to support standard Kubernetes object manipulation.
  - Generates a versioned, typed **clientset**, along with **listers** and **informers**, providing a native `client-go` experience for consumers.

---
