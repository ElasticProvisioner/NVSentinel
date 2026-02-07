# NVML Provider Refactoring & Old Code Removal — Design

## Problem

PR #720 introduced a new architecture (`pkg/controlplane/apiserver/`, `pkg/services/device/v1alpha1/`,
`pkg/storage/memory/`) but the old `pkg/deviceapiserver/` code (server.go, cache/, service/, metrics/)
still exists. The NVML provider test infrastructure (`mock_test.go`, `provider_test.go`) imports the
old `cache` package, preventing its deletion.

## Goal

Remove all old `pkg/deviceapiserver/` infrastructure, relocate the NVML provider to
`pkg/providers/nvml/`, rewire `cmd/device-api-server/main.go` to the new controlplane server,
and replace cache-based test fakes with a shared bufconn gRPC test server.

## Approach: Relocate + Shared Test Server (Option A)

### Package Layout After Refactoring

```
pkg/
├── controlplane/apiserver/       # Server (exists)
├── services/device/v1alpha1/     # GPU gRPC service (exists)
├── storage/memory/               # In-memory storage.Interface (exists)
├── providers/
│   └── nvml/                     # MOVED from pkg/deviceapiserver/nvml/
├── testutil/
│   └── grpcserver.go             # NEW: shared bufconn test server helper
│
├── deviceapiserver/              # DELETED entirely

cmd/
├── device-api-server/main.go     # MODIFIED: use controlplane server
├── nvml-provider/main.go         # MODIFIED: import path update
├── nvml-provider/reconciler.go   # MODIFIED: import path update
```

### Shared Test Helper

`pkg/testutil/grpcserver.go` provides:

```go
func NewTestGPUClient(t *testing.T) v1alpha1.GpuServiceClient
```

Internally: bufconn listener -> gRPC server -> GPUServiceProvider.Install() -> gRPC client.
Same pattern as `integration_test.go`, extracted for reuse.

### NVML Test Refactoring

The `fakeGpuServiceClient` in `mock_test.go` (which wraps `cache.GpuCache`) is replaced by
a real gRPC client from `testutil.NewTestGPUClient(t)`.

Direct cache inspection (`gpuCache.Count()`, `gpuCache.Get()`, `gpuCache.List()`) in
`provider_test.go` is replaced by gRPC client calls (`ListGpus`, `GetGpu`).

NVML hardware mocks (`MockLibrary`, `MockDevice`, `MockEventSet`) are unchanged.

### Rewiring cmd/device-api-server/main.go

Replace `deviceapiserver.New(config, logger)` with the controlplane server startup pipeline:

```
options.NewOptions() -> AddFlags() -> Parse() -> Complete() -> Validate()
-> apiserver.NewConfig() -> Complete() -> New(storage{InMemory:true})
-> PrepareRun() -> Run()
```

Service providers auto-register via `init()` in `gpu_provider.go`.

CLI flag names change from port-based (`--health-port`) to address-based
(`--health-probe-bind-address`). Expected breaking change on a feature branch.

### Deletion Inventory

**Delete entirely:**
- `pkg/deviceapiserver/server.go`
- `pkg/deviceapiserver/cache/`
- `pkg/deviceapiserver/service/`
- `pkg/deviceapiserver/metrics/`
- `pkg/deviceapiserver/config.go`

**Move:**
- `pkg/deviceapiserver/nvml/` -> `pkg/providers/nvml/`

**Create:**
- `pkg/testutil/grpcserver.go`

**Modify:**
- `pkg/providers/nvml/mock_test.go` — delete fakeGpuServiceClient, use testutil
- `pkg/providers/nvml/provider_test.go` — replace cache assertions with gRPC assertions
- `cmd/device-api-server/main.go` — rewire to controlplane server
- `cmd/nvml-provider/main.go` — update import path
- `cmd/nvml-provider/reconciler.go` — update import path
- `pkg/services/device/v1alpha1/integration_test.go` — use shared testutil helper
