# Device API Server - Implementation Tasks

> Detailed task breakdown for implementation tracking

## Legend

- **Size**: S (< 2h), M (2-4h), L (4-8h), XL (> 8h)
- **Status**: ⬜ TODO | 🔄 In Progress | ✅ Done | ⏸️ Blocked

---

## Phase 1: Core Server Foundation

### P1.1 Project Scaffolding [S]
**Status**: ✅ Done

**Tasks**:
- [x] Create `cmd/device-api-server/main.go`
- [x] Create `pkg/deviceapiserver/` package structure
- [x] Add `go.mod` for server module
- [x] Add build target to `Makefile`
- [x] Configure `.golangci.yml` for new code paths

**Files created**:
```
cmd/device-api-server/
├── main.go
pkg/deviceapiserver/
├── server.go
├── config.go
├── cache/
│   ├── cache.go
│   └── broadcaster.go
├── service/
│   ├── consumer.go
│   └── provider.go
```

**Acceptance Criteria**:
- [x] `go build ./cmd/device-api-server` succeeds
- [x] `make build` includes server binary

---

### P1.2 Proto Extensions [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `api/proto/device/v1alpha1/provider.proto`
- [x] Define `ProviderService` with RPCs:
  - `RegisterGpu(RegisterGpuRequest) returns (RegisterGpuResponse)`
  - `UnregisterGpu(UnregisterGpuRequest) returns (UnregisterGpuResponse)`
  - `UpdateGpuStatus(UpdateGpuStatusRequest) returns (UpdateGpuStatusResponse)`
  - `UpdateGpuCondition(UpdateGpuConditionRequest) returns (UpdateGpuConditionResponse)`
- [x] Add `resource_version` field to `Gpu` message
- [x] Regenerate Go code: `make protos-generate`
- [x] Verify generated code compiles

**Proto additions**:
```protobuf
// provider.proto
service ProviderService {
  rpc RegisterGpu(RegisterGpuRequest) returns (RegisterGpuResponse);
  rpc UnregisterGpu(UnregisterGpuRequest) returns (UnregisterGpuResponse);
  rpc UpdateGpuStatus(UpdateGpuStatusRequest) returns (UpdateGpuStatusResponse);
  rpc UpdateGpuCondition(UpdateGpuConditionRequest) returns (UpdateGpuConditionResponse);
}
```

**Acceptance Criteria**:
- Proto compiles without errors
- Generated Go interfaces available

---

### P1.3 Cache Implementation [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/cache/cache.go`
- [x] Implement `GpuCache` struct with `sync.RWMutex`
- [x] Implement methods:
  - `Get(uuid string) (*v1alpha1.Gpu, bool)`
  - `List() []*v1alpha1.Gpu`
  - `Set(gpu *v1alpha1.Gpu) int64` (returns resource_version)
  - `Delete(uuid string) bool`
  - `UpdateCondition(uuid, providerID string, condition *v1alpha1.Condition) (int64, error)`
- [x] Implement resource version tracking
- [x] Add metrics via callback hooks

**Key Implementation**:
```go
type GpuCache struct {
    mu              sync.RWMutex
    gpus            map[string]*cachedGpu
    resourceVersion int64
    broadcaster     *Broadcaster
}

type cachedGpu struct {
    gpu             *v1alpha1.Gpu
    resourceVersion int64
}

// Get acquires read lock - blocks if write pending
func (c *GpuCache) Get(name string) (*v1alpha1.Gpu, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    cached, ok := c.gpus[name]
    if !ok {
        return nil, false
    }
    return proto.Clone(cached.gpu).(*v1alpha1.Gpu), true
}

// UpdateStatus acquires write lock - blocks all readers
func (c *GpuCache) UpdateStatus(name string, status *v1alpha1.GpuStatus) (int64, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    cached, ok := c.gpus[name]
    if !ok {
        return 0, ErrGpuNotFound
    }
    
    cached.gpu.Status = status
    c.resourceVersion++
    cached.resourceVersion = c.resourceVersion
    
    c.broadcaster.Notify(WatchEvent{Type: "MODIFIED", Object: cached.gpu})
    
    return c.resourceVersion, nil
}
```

**Acceptance Criteria**:
- Unit tests pass for all cache operations
- Concurrent read/write test verifies blocking behavior
- No data races detected by `-race`

---

### P1.4 Watch Broadcaster [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/cache/broadcaster.go`
- [x] Implement subscription management
- [x] Implement fan-out to all subscribers
- [x] Handle slow consumers (bounded buffer, drop policy)
- [x] Implement cleanup on subscriber disconnect

**Key Implementation**:
```go
type Broadcaster struct {
    mu          sync.RWMutex
    subscribers map[string]chan WatchEvent
    bufferSize  int
}

type WatchEvent struct {
    Type   string // ADDED, MODIFIED, DELETED, ERROR
    Object *v1alpha1.Gpu
}

func (b *Broadcaster) Subscribe(id string) <-chan WatchEvent {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    ch := make(chan WatchEvent, b.bufferSize)
    b.subscribers[id] = ch
    return ch
}

func (b *Broadcaster) Unsubscribe(id string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    if ch, ok := b.subscribers[id]; ok {
        close(ch)
        delete(b.subscribers, id)
    }
}

func (b *Broadcaster) Notify(event WatchEvent) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    
    for id, ch := range b.subscribers {
        select {
        case ch <- event:
            // sent
        default:
            // buffer full, log warning
            klog.V(2).InfoS("Watch buffer full, dropping event", "subscriber", id)
        }
    }
}
```

**Acceptance Criteria**:
- Multiple subscribers receive events
- Slow subscriber doesn't block others
- Cleanup on unsubscribe

---

### P1.5 Consumer gRPC Service [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/service/consumer.go`
- [x] Implement `GpuServiceServer` interface
- [x] Implement `GetGpu` - single GPU lookup
- [x] Implement `ListGpus` - all GPUs
- [x] Implement `WatchGpus` - streaming with broadcaster
- [x] Add request validation
- [x] Add logging with klog/v2

**Key Implementation**:
```go
type ConsumerService struct {
    v1alpha1.UnimplementedGpuServiceServer
    cache  *cache.GpuCache
    logger klog.Logger
}

func (s *ConsumerService) GetGpu(ctx context.Context, req *v1alpha1.GetGpuRequest) (*v1alpha1.GetGpuResponse, error) {
    logger := klog.FromContext(ctx).WithValues("method", "GetGpu", "name", req.Name)
    
    gpu, ok := s.cache.Get(req.Name)
    if !ok {
        logger.V(1).Info("GPU not found")
        return nil, status.Error(codes.NotFound, "gpu not found")
    }
    
    logger.V(2).Info("GPU retrieved successfully")
    return &v1alpha1.GetGpuResponse{Gpu: gpu}, nil
}

func (s *ConsumerService) WatchGpus(req *v1alpha1.WatchGpusRequest, stream v1alpha1.GpuService_WatchGpusServer) error {
    ctx := stream.Context()
    logger := klog.FromContext(ctx).WithValues("method", "WatchGpus")
    
    id := uuid.New().String()
    events := s.cache.Subscribe(id)
    defer s.cache.Unsubscribe(id)
    
    // Send initial state
    for _, gpu := range s.cache.List() {
        if err := stream.Send(&v1alpha1.WatchGpusResponse{
            Type:   "ADDED",
            Object: gpu,
        }); err != nil {
            return err
        }
    }
    
    // Stream updates
    for {
        select {
        case <-ctx.Done():
            logger.V(1).Info("Watch stream closed by client")
            return nil
        case event, ok := <-events:
            if !ok {
                return nil
            }
            if err := stream.Send(&v1alpha1.WatchGpusResponse{
                Type:   event.Type,
                Object: event.Object,
            }); err != nil {
                return err
            }
        }
    }
}
```

**Acceptance Criteria**:
- GetGpu returns GPU or NotFound
- ListGpus returns all cached GPUs
- WatchGpus streams initial state + updates

---

### P1.6 Provider gRPC Service [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/service/provider.go`
- [x] Implement `ProviderServiceServer` interface
- [x] Implement `RegisterGpu`
- [x] Implement `UnregisterGpu`
- [x] Implement `UpdateGpuStatus`
- [x] Implement `UpdateGpuCondition`
- [x] Add request validation
- [x] Add logging with klog/v2

**Key Implementation**:
```go
type ProviderService struct {
    v1alpha1.UnimplementedProviderServiceServer
    cache  *cache.GpuCache
    logger klog.Logger
}

func (s *ProviderService) UpdateGpuStatus(ctx context.Context, req *v1alpha1.UpdateGpuStatusRequest) (*v1alpha1.UpdateGpuStatusResponse, error) {
    logger := klog.FromContext(ctx).WithValues("method", "UpdateGpuStatus", "name", req.Name)
    
    // This acquires write lock, blocking all consumer reads
    version, err := s.cache.UpdateStatus(req.Name, req.Status)
    if err != nil {
        if errors.Is(err, cache.ErrGpuNotFound) {
            return nil, status.Error(codes.NotFound, "gpu not found")
        }
        logger.Error(err, "Failed to update GPU status")
        return nil, status.Error(codes.Internal, "internal error")
    }
    
    logger.V(1).Info("GPU status updated", "resourceVersion", version)
    return &v1alpha1.UpdateGpuStatusResponse{ResourceVersion: version}, nil
}
```

**Acceptance Criteria**:
- RegisterGpu creates new GPU or returns existing
- UpdateGpuStatus blocks readers during update
- Changes trigger WatchGpus notifications

---

### P1.7 Server Assembly & Graceful Shutdown [S]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/server.go` - assembles all components
- [x] Create gRPC server with interceptors
- [x] Register consumer and provider services
- [x] Implement signal handling (SIGTERM, SIGINT)
- [x] Implement graceful shutdown sequence
- [x] Add startup logging

**Key Implementation**:
```go
type Server struct {
    grpcServer *grpc.Server
    cache      *cache.GpuCache
    listener   net.Listener
    logger     klog.Logger
}

func (s *Server) Run(ctx context.Context) error {
    // Start gRPC server
    go func() {
        s.logger.Info("Starting gRPC server", "address", s.listener.Addr())
        if err := s.grpcServer.Serve(s.listener); err != nil {
            s.logger.Error(err, "gRPC server error")
        }
    }()
    
    // Wait for shutdown signal
    <-ctx.Done()
    s.logger.Info("Shutting down server")
    
    // Graceful stop with timeout
    stopped := make(chan struct{})
    go func() {
        s.grpcServer.GracefulStop()
        close(stopped)
    }()
    
    select {
    case <-stopped:
        s.logger.Info("Server stopped gracefully")
    case <-time.After(30 * time.Second):
        s.logger.Info("Forcing server stop")
        s.grpcServer.Stop()
    }
    
    return nil
}
```

**Acceptance Criteria**:
- Server starts and accepts connections
- SIGTERM triggers graceful shutdown
- In-flight requests complete before exit

---

### P1.8 Unit Tests [L]
**Status**: ✅ Done

**Tasks**:
- [x] Cache tests
  - [x] Test concurrent reads
  - [x] Test read blocking during write
  - [x] Test resource version increments
- [x] Broadcaster tests
  - [x] Test multiple subscribers
  - [x] Test slow subscriber handling
- [x] Consumer service tests
  - [x] Test GetGpu success/not found
  - [x] Test ListGpus
  - [x] Test WatchGpus stream
- [x] Provider service tests
  - [x] Test RegisterGpu
  - [x] Test UpdateGpuStatus
  - [x] Test UpdateGpuCondition
- [x] Integration test: full flow
  - [x] Provider registers GPU
  - [x] Consumer watches
  - [x] Provider updates status
  - [x] Consumer receives update

**Test Files Created**:
- `pkg/deviceapiserver/cache/cache_test.go` - Cache unit tests
- `pkg/deviceapiserver/cache/broadcaster_test.go` - Broadcaster tests  
- `pkg/deviceapiserver/service/consumer_test.go` - Consumer service tests
- `pkg/deviceapiserver/service/provider_test.go` - Provider service tests
- `pkg/deviceapiserver/service/integration_test.go` - Full integration tests

**Key Test - Blocking Behavior**:
```go
func TestCache_ReadBlocksDuringWrite(t *testing.T) {
    c := cache.New()
    c.Set(&v1alpha1.Gpu{Name: "gpu-0"})
    
    var wg sync.WaitGroup
    readStarted := make(chan struct{})
    writeStarted := make(chan struct{})
    readCompleted := make(chan struct{})
    
    // Start a write that holds the lock
    wg.Add(1)
    go func() {
        defer wg.Done()
        c.WithWriteLock(func() {
            close(writeStarted)
            time.Sleep(100 * time.Millisecond) // Hold lock
        })
    }()
    
    <-writeStarted // Wait for write to acquire lock
    
    // Start a read - should block
    wg.Add(1)
    go func() {
        defer wg.Done()
        close(readStarted)
        _, _ = c.Get("gpu-0") // Should block here
        close(readCompleted)
    }()
    
    <-readStarted
    
    // Verify read hasn't completed yet
    select {
    case <-readCompleted:
        t.Fatal("Read completed while write lock was held")
    case <-time.After(50 * time.Millisecond):
        // Expected - read is blocked
    }
    
    wg.Wait()
    
    // Now read should have completed
    select {
    case <-readCompleted:
        // Expected
    default:
        t.Fatal("Read did not complete after write released")
    }
}
```

**Acceptance Criteria**:
- All tests pass
- Test coverage > 80%
- No race conditions detected

---

## Phase 2: Kubernetes Integration

### P2.1 klog/v2 Integration [M]
**Status**: ✅ Done

**Tasks**:
- [x] Initialize klog in main.go
- [x] Configure log verbosity flag
- [x] Configure JSON output format
- [x] Use contextual loggers throughout
- [x] Add component name to all logs

**Key Implementation**:
```go
func main() {
    klog.InitFlags(nil)
    flag.Parse()
    
    // Configure JSON format for production
    config := textlogger.NewConfig()
    if os.Getenv("LOG_FORMAT") == "json" {
        config = textlogger.NewConfig(textlogger.WithFormat(textlogger.FormatJSON))
    }
    logger := textlogger.NewLogger(config).WithName("device-api-server")
    klog.SetLogger(logger)
    
    ctx := klog.NewContext(context.Background(), logger)
    
    // Use logger throughout
    klog.FromContext(ctx).Info("Starting server", "version", version)
}
```

---

### P2.2 Health Probes [M]
**Status**: ✅ Done

**Tasks**:
- [x] Implement gRPC health protocol
- [x] Create HTTP health server
- [x] Implement `/healthz` (liveness)
- [x] Implement `/readyz` (readiness)
- [x] Track server readiness state
- [x] Update health on shutdown

**Key Implementation**:
```go
type HealthServer struct {
    ready    atomic.Bool
    httpSrv  *http.Server
    grpcHealth *health.Server
}

func (h *HealthServer) SetReady(ready bool) {
    h.ready.Store(ready)
    if ready {
        h.grpcHealth.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
    } else {
        h.grpcHealth.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
    }
}

func (h *HealthServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

func (h *HealthServer) handleReadyz(w http.ResponseWriter, r *http.Request) {
    if h.ready.Load() {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("not ready"))
    }
}
```

---

### P2.3 Configuration [S]
**Status**: ✅ Done

**Tasks**:
- [x] Define `Config` struct
- [x] Add command-line flags
- [x] Add environment variable support
- [x] Validate configuration
- [x] Log effective configuration

**Options**:
```go
type Options struct {
    GRPCAddress    string
    UnixSocket     string
    HealthPort     int
    MetricsPort    int
    LogVerbosity   int
    LogFormat      string
    ShutdownTimeout time.Duration
}
```

---

### P2.4 Unix Socket Support [S]
**Status**: ✅ Done

**Tasks**:
- [x] Create socket directory if needed
- [x] Remove stale socket file on startup
- [x] Listen on Unix socket
- [x] Set appropriate permissions
- [x] Support both TCP and Unix listeners

---

## Phase 3: NVML Fallback Provider

> **Design Document**: [nvml-fallback-provider.md](./nvml-fallback-provider.md)

### P3.1 NVML Integration [M]
**Status**: ✅ Done

**Tasks**:
- [x] Add `github.com/NVIDIA/go-nvml/pkg/nvml` dependency
- [x] Create `pkg/deviceapiserver/nvml/provider.go`
- [x] Implement `NVMLProvider` struct with lifecycle management
- [x] Implement NVML library path discovery

**Key Implementation**:
```go
type NVMLProvider struct {
    nvmllib   nvml.Interface
    eventSet  nvml.EventSet
    cache     *cache.GpuCache
    logger    klog.Logger
    ctx       context.Context
    cancel    context.CancelFunc
    wg        sync.WaitGroup
}

func NewNVMLProvider(cache *cache.GpuCache, driverRoot string) (*NVMLProvider, error) {
    libraryPath := findDriverLibrary(driverRoot)
    nvmllib := nvml.New(nvml.WithLibraryPath(libraryPath))
    return &NVMLProvider{nvmllib: nvmllib, cache: cache}, nil
}
```

**Acceptance Criteria**:
- NVML library loaded successfully when available
- Graceful error when NVML unavailable

---

### P3.2 Device Enumeration [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/nvml/enumerator.go`
- [x] Implement GPU enumeration via NVML
- [x] Extract device info (UUID, product name, memory)
- [ ] Handle MIG device enumeration (deferred - proto extension needed)
- [x] Register enumerated devices in cache with `NVMLReady` condition

**Acceptance Criteria**:
- All GPUs on node discovered at startup
- GPU info matches `nvidia-smi` output
- MIG devices enumerated when MIG mode enabled

---

### P3.3 XID Event Monitoring [M]
**Status**: ✅ Done

**Tasks**:
- [x] Create `pkg/deviceapiserver/nvml/health_monitor.go`
- [x] Create NVML event set for health events
- [x] Register for `XidCriticalError`, `DoubleBitEccError`, `SingleBitEccError`
- [x] Implement event loop with 5s timeout
- [x] Create `pkg/deviceapiserver/nvml/xid.go` with ignored XIDs list
- [x] Update GPU condition on critical XID events

**Ignored XIDs** (application errors, not hardware):
```go
var ignoredXids = map[uint64]bool{
    13:  true, // Graphics Engine Exception
    31:  true, // GPU memory page fault
    43:  true, // GPU stopped processing
    45:  true, // Preemptive cleanup
    68:  true, // Video processor exception
    109: true, // Context Switch Timeout
}
```

**Acceptance Criteria**:
- Critical XID errors mark GPU as unhealthy
- Ignored XIDs don't affect GPU health
- GPU lost event marks all GPUs unhealthy

---

### P3.4 Server Integration [S]
**Status**: ✅ Done

**Tasks**:
- [x] Add NVML config fields to `Config` struct
- [x] Add CLI flags: `--enable-nvml-provider`, `--nvml-driver-root`, `--nvml-ignored-xids`
- [x] Initialize NVML provider in server startup (graceful on failure)
- [x] Stop NVML provider in server shutdown
- [x] Add NVML status to `/metrics` endpoint

**Acceptance Criteria**:
- Server starts even if NVML unavailable
- NVML provider properly cleaned up on shutdown
- Metrics show NVML provider status

---

### P3.5 Helm Chart Updates [M]
**Status**: ✅ Done

**Tasks**:
- [x] Add `nvml.enabled` value (default: false)
- [x] Add `runtimeClassName` value (default: "nvidia")
- [x] Add `NVIDIA_VISIBLE_DEVICES=all` env var when NVML enabled
- [x] Add `NVIDIA_DRIVER_CAPABILITIES=utility` env var
- [x] Update DaemonSet template with RuntimeClass
- [ ] Document RuntimeClass prerequisite (deferred to Phase 6)

**values.yaml additions**:
```yaml
nvml:
  enabled: true
  driverRoot: /run/nvidia/driver
  additionalIgnoredXids: ""
runtimeClassName: nvidia
```

**Acceptance Criteria**:
- Helm install with NVML works on GPU nodes
- No GPU resources consumed
- Clear error if RuntimeClass missing

---

### P3.6 Unit Tests [L]
**Status**: ✅ Done

**Tasks**:
- [x] Mock NVML interface for testing
- [x] Test device enumeration with mock
- [x] Test XID event handling
- [x] Test graceful degradation (NVML unavailable)
- [x] Test ignored XIDs filtering
- [x] Test condition updates

**Test Files Created**:
- `pkg/deviceapiserver/nvml/interface.go` - Defines testable Library, Device, EventSet interfaces
- `pkg/deviceapiserver/nvml/mock_test.go` - Mock implementations for testing
- `pkg/deviceapiserver/nvml/provider_test.go` - Provider unit tests (12 tests)
- `pkg/deviceapiserver/nvml/xid_test.go` - XID utility tests (10 tests)

**Key Tests**:
- `TestProvider_Start_Success` - Verifies successful provider initialization
- `TestProvider_Start_NVMLInitFails` - Tests graceful degradation when NVML unavailable
- `TestProvider_Start_NoGPUs` - Tests handling of nodes without GPUs
- `TestProvider_DeviceEnumeration` - Tests device enumeration with mock devices
- `TestProvider_DeviceEnumeration_PartialFailure` - Tests partial device failures
- `TestProvider_HealthCheckDisabled` - Tests health monitoring disabled mode
- `TestProvider_UpdateCondition` - Tests condition updates
- `TestIsIgnoredXid_*` - Tests XID filtering logic
- `TestParseIgnoredXids` - Tests XID parsing from string

**Acceptance Criteria**:
- All NVML provider code has unit tests ✅
- Tests pass without real GPU/NVML ✅
- 22 tests total, all passing with `-race` flag

---

## Phase 4: Observability

### P4.1-P4.4 Prometheus Metrics [L]
**Status**: ✅ Done

**Implementation**:
- Created `pkg/deviceapiserver/metrics/metrics.go` with `prometheus/client_golang`
- Replaced manual text-based metrics with proper Prometheus registry
- Added background metrics updater (5s interval)

**Metrics Implemented**:
- Server info: `device_api_server_info{version,go_version,node}`
- Cache gauges: `device_api_server_cache_gpus_total`, `_healthy`, `_unhealthy`, `_unknown`, `_resource_version`
- Cache counter: `device_api_server_cache_updates_total{operation}` (register, unregister, update_status, update_condition, set)
- Watch gauges: `device_api_server_watch_streams_active`
- Watch counter: `device_api_server_watch_events_total{type}` (ADDED, MODIFIED, DELETED)
- NVML gauges: `device_api_server_nvml_provider_enabled`, `_gpu_count`, `_health_monitor_running`
- gRPC metrics: `device_api_server_grpc_requests_total{method,code}`, `_request_duration_seconds{method}`
- Standard Go/process collectors included

**Test Coverage**:
- `pkg/deviceapiserver/metrics/metrics_test.go` - 7 tests covering all metric operations

---

### P4.5 Alerting Rules [M]
**Status**: ✅ Done

**Implementation**: Updated `charts/device-api-server/templates/prometheusrule.yaml`

**Alerts Defined**:
- `DeviceAPIServerDown` - Server unreachable for 5m (critical)
- `DeviceAPIServerHighLatency` - P99 latency > 500ms for 5m (warning)
- `DeviceAPIServerHighErrorRate` - Error rate > 10% for 5m (warning)
- `DeviceAPIServerUnhealthyGPUs` - Any unhealthy GPUs detected (warning)
- `DeviceAPIServerNoGPUs` - No GPUs registered for 10m (warning)
- `DeviceAPIServerNVMLProviderDown` - NVML provider not running for 5m (warning)
- `DeviceAPIServerNVMLHealthMonitorDown` - Health monitor not running for 5m (warning)
- `DeviceAPIServerHighMemory` - Memory > 512MB for 10m (warning)
            
        - alert: DeviceAPIServerUnhealthyGPUs
          expr: device_api_server_cache_gpus_unhealthy > 0
          for: 1m
          labels:
            severity: warning
          annotations:
            summary: "{{ $value }} unhealthy GPUs detected"
            
        - alert: DeviceAPIServerHighErrorRate
          expr: |
            rate(grpc_server_handled_total{
              grpc_code!="OK",
              grpc_service="nvidia.device.v1alpha1.GpuService"
            }[5m]) > 0.1
          for: 5m
          labels:
            severity: warning
```

---

## Phase 5: Helm Chart

### P5.1-P5.10 Complete Helm Chart [XL]
**Status**: ✅ Done

**Templates Implemented**:
- `Chart.yaml` - Complete with metadata, keywords, maintainers
- `values.yaml` - Comprehensive configuration (226 lines)
- `templates/_helpers.tpl` - Helper functions
- `templates/daemonset.yaml` - DaemonSet with NVML support, probes, security
- `templates/serviceaccount.yaml` - ServiceAccount
- `templates/service.yaml` - Metrics service
- `templates/servicemonitor.yaml` - Prometheus Operator integration
- `templates/prometheusrule.yaml` - 8 alerting rules
- `templates/NOTES.txt` - Post-install instructions

**Chart Features**:
- NVML fallback provider support with automatic RuntimeClass
- Prometheus ServiceMonitor and PrometheusRule
- Configurable node selector and tolerations
- Security contexts (non-root, read-only rootfs)
- Graceful shutdown with preStop hook
- Customizable resources, probes, and logging

**Documentation**:
- `README.md` - Comprehensive chart documentation with:
  - Installation instructions
  - Configuration reference
  - Metrics and alerting documentation
  - Client connection examples
  - Troubleshooting guide

**Validation**:
```bash
$ helm lint ./charts/device-api-server
==> Linting ./charts/device-api-server
1 chart(s) linted, 0 chart(s) failed
```

---

## Phase 6: Documentation & Polish

### P6.1-P6.7 Documentation [L]
**Status**: ✅ Done

**Documents created**:
- [x] `docs/api/device-api-server.md` - Complete API reference
  - GpuService methods (GetGpu, ListGpus, WatchGpus)
  - ProviderService methods (RegisterGpu, UpdateGpuStatus, etc.)
  - Resource types (Gpu, GpuSpec, GpuStatus, Condition)
  - Go client examples
  - Error codes
- [x] `docs/operations/device-api-server.md` - Operations guide
  - Architecture overview
  - Deployment instructions
  - Configuration reference
  - NVML provider documentation
  - Prometheus metrics reference
  - Alerting rules
  - Troubleshooting guide
  - Security considerations
- [x] `charts/device-api-server/README.md` - Helm chart docs (Phase 5)
- [x] Main `README.md` updated with:
  - Project overview and architecture diagram
  - Quick start (Helm + Go client)
  - API overview (GpuService + ProviderService)
  - Development commands
  - Documentation links

---

## Dependency Graph

```
P1.1 ─────────────────────────────────────────────────────────────────►
      │
      ▼
P1.2 ─────────────────────────────────────────────────────────────────►
      │
      ├──────────────────────────────────────────────────────┐
      ▼                                                      ▼
P1.3 ──────────────────────────────────► P1.4 ──────────────────────►
      │                                   │
      │                                   │
      ├───────────────────────────────────┤
      ▼                                   ▼
P1.5 ──────────────────────────────────► P1.6 ──────────────────────►
      │                                   │
      │                                   │
      ├───────────────────────────────────┤
      ▼                                   │
P1.7 ◄────────────────────────────────────┤
      │                                   │
      ▼                                   ▼
P1.8 ◄──────────────────────────────────────────────────────────────
      │
      ▼
P2.* ─────────────────────────────────────────────────────────────────►
      │
      ▼
P3.* ─────────────────────────────────────────────────────────────────►
      (NVML Fallback Provider)
      │
      ▼
P4.* ─────────────────────────────────────────────────────────────────►
      (Observability)
      │
      ▼
P5.* ─────────────────────────────────────────────────────────────────►
      (Helm Chart)
      │
      ▼
P6.* ─────────────────────────────────────────────────────────────────►
      (Documentation & Polish)
```

---

## Quick Start Commands

```bash
# Build server
make build-server

# Run locally
./bin/device-api-server --grpc-address=:50051 --health-port=8081

# Run tests
go test ./internal/server/...

# Build Helm chart
helm package charts/device-api-server

# Install
helm install device-api-server ./charts/device-api-server \
  --namespace device-api \
  --create-namespace
```

---

*Last updated: 2026-01-21 (Phase 6 Documentation complete)*
