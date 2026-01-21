# NVML Fallback Provider - Design Document

> **Status**: Draft  
> **Author**: NVSentinel Team  
> **Created**: 2026-01-21  
> **Related**: [device-api-server.md](./device-api-server.md)

## Table of Contents

- [Executive Summary](#executive-summary)
- [Motivation](#motivation)
- [Architecture](#architecture)
- [NVML Integration](#nvml-integration)
- [Health Condition Model](#health-condition-model)
- [Deployment Configuration](#deployment-configuration)
- [Implementation Plan](#implementation-plan)
- [Risk Assessment](#risk-assessment)

---

## Executive Summary

This document describes an enhancement to the Device API Server that adds a **built-in NVML-based health provider** as a fallback mechanism. When no external gRPC providers (like NVSentinel health monitors) are connected, the server can use NVML directly to:

1. **Enumerate GPUs** on the node at startup
2. **Monitor GPU health** via XID error events
3. **Provide baseline device information** to consumers

This ensures the Device API Server always has device information available, even in minimal deployments without dedicated health monitoring infrastructure.

### Key Design Principles

| Principle | Description |
|-----------|-------------|
| **No GPU resource consumption** | Uses RuntimeClass injection, not `nvidia.com/gpu` resources |
| **Optional feature** | Disabled by default, enabled via flag |
| **Graceful degradation** | Server starts even if NVML unavailable |
| **Provider coexistence** | External providers can override/supplement NVML data |

---

## Motivation

### Problem Statement

The current Device API Server design requires external providers to populate GPU state:

```
┌─────────────────┐                    ┌─────────────────┐
│  Health Monitor │───RegisterGpu()───►│ Device API      │
│  (External)     │───UpdateStatus()──►│ Server          │
└─────────────────┘                    └─────────────────┘
```

**Issues with this approach:**

1. **Bootstrap problem**: Consumers get empty results until providers connect
2. **Deployment complexity**: Requires deploying separate health monitor component
3. **Single point of failure**: If health monitor fails, no GPU data available
4. **Delayed visibility**: GPU info not available until provider initialization

### Solution: Built-in NVML Fallback

```
┌─────────────────────────────────────────────────────────────────┐
│                      Device API Server                           │
│  ┌─────────────────────┐    ┌─────────────────────────────────┐ │
│  │  Built-in NVML      │    │   External gRPC Providers       │ │
│  │  Provider (fallback)│    │   (optional, higher priority)   │ │
│  └──────────┬──────────┘    └───────────────┬─────────────────┘ │
│             │                                │                   │
│             └────────────┬───────────────────┘                   │
│                          ▼                                       │
│                    ┌──────────┐                                  │
│                    │  Cache   │                                  │
│                    └──────────┘                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Benefits:**

- Immediate GPU visibility on server startup
- Zero additional deployment requirements
- Baseline health monitoring always available
- External providers can add richer health data when deployed

---

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Device API Server                                   │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                         Provider Layer                                      │ │
│  │                                                                             │ │
│  │   ┌─────────────────────────────┐    ┌─────────────────────────────────┐   │ │
│  │   │    Built-in NVML Provider   │    │   External gRPC Providers       │   │ │
│  │   │                             │    │   (ProviderService)             │   │ │
│  │   │  ┌───────────────────────┐  │    │                                 │   │ │
│  │   │  │   Device Enumerator   │  │    │   - NVSentinel Health Monitor   │   │ │
│  │   │  │   - GPU discovery     │  │    │   - DCGM-based monitors         │   │ │
│  │   │  │   - MIG enumeration   │  │    │   - Custom health checks        │   │ │
│  │   │  └───────────────────────┘  │    │                                 │   │ │
│  │   │                             │    │   Condition types:              │   │ │
│  │   │  ┌───────────────────────┐  │    │   - DCGMHealth                  │   │ │
│  │   │  │   Health Monitor      │  │    │   - NVSentinelHealth            │   │ │
│  │   │  │   - XID events        │  │    │   - CustomHealth                │   │ │
│  │   │  │   - ECC errors        │  │    │                                 │   │ │
│  │   │  └───────────────────────┘  │    └─────────────────────────────────┘   │ │
│  │   │                             │                    │                      │ │
│  │   │  Condition type:            │                    │                      │ │
│  │   │  - NVMLReady                │                    │                      │ │
│  │   └──────────────┬──────────────┘                    │                      │ │
│  │                  │                                   │                      │ │
│  │                  └─────────────┬─────────────────────┘                      │ │
│  │                                ▼                                            │ │
│  │                  ┌─────────────────────────────┐                            │ │
│  │                  │         GPU Cache           │                            │ │
│  │                  │   - Condition aggregation   │                            │ │
│  │                  │   - Resource versioning     │                            │ │
│  │                  │   - Watch notifications     │                            │ │
│  │                  └─────────────────────────────┘                            │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                         Consumer Layer                                      │ │
│  │   GetGpu() | ListGpus() | WatchGpus()                                       │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
Startup Sequence (NVML enabled):
═════════════════════════════════

1. Server starts
   │
2. NVML Provider initializes
   │
   ├─► nvml.Init()
   │   │
   │   ├─► Success: enumerate GPUs, register in cache
   │   │             set NVMLReady=True condition
   │   │
   │   └─► Failure: log warning, continue without NVML
   │                (cache remains empty until providers connect)
   │
3. Start XID event monitoring (if NVML init succeeded)
   │
4. Server ready for consumers
   │
5. External providers connect (optional)
   │
   └─► Add/update conditions on existing GPUs
       or register new GPUs


Runtime Event Flow:
═══════════════════

  NVML Event (XID error)              External Provider Update
          │                                     │
          ▼                                     ▼
  ┌───────────────┐                   ┌───────────────────────┐
  │ Health Monitor│                   │ ProviderService.      │
  │ goroutine     │                   │ UpdateGpuCondition()  │
  └───────┬───────┘                   └───────────┬───────────┘
          │                                       │
          │ UpdateCondition(                      │ UpdateCondition(
          │   type: "NVMLReady",                  │   type: "DCGMHealth",
          │   status: "False",                    │   status: "False",
          │   reason: "XidError",                 │   reason: "ThermalThrottle",
          │   message: "XID 79")                  │   message: "GPU temp > 90C")
          │                                       │
          └───────────────┬───────────────────────┘
                          ▼
                    ┌──────────┐
                    │  Cache   │──► Watch notifications
                    └──────────┘
```

---

## NVML Integration

### Library Access Pattern

Following the pattern from [NVIDIA/k8s-dra-driver-gpu](https://github.com/NVIDIA/k8s-dra-driver-gpu), we use NVML with explicit library path:

```go
import "github.com/NVIDIA/go-nvml/pkg/nvml"

type NVMLProvider struct {
    nvmllib   nvml.Interface
    eventSet  nvml.EventSet
    cache     *cache.GpuCache
    logger    klog.Logger
    
    // Lifecycle
    ctx       context.Context
    cancel    context.CancelFunc
    wg        sync.WaitGroup
}

func NewNVMLProvider(cache *cache.GpuCache, driverRoot string) (*NVMLProvider, error) {
    // Locate driver library (injected via RuntimeClass)
    libraryPath := findDriverLibrary(driverRoot)
    
    nvmllib := nvml.New(
        nvml.WithLibraryPath(libraryPath),
    )
    
    return &NVMLProvider{
        nvmllib: nvmllib,
        cache:   cache,
    }, nil
}
```

### Device Enumeration

On startup, enumerate all GPUs and MIG devices:

```go
func (p *NVMLProvider) EnumerateDevices() error {
    if ret := p.nvmllib.Init(); ret != nvml.SUCCESS {
        return fmt.Errorf("NVML init failed: %v", ret)
    }
    defer p.nvmllib.Shutdown()
    
    count, ret := p.nvmllib.DeviceGetCount()
    if ret != nvml.SUCCESS {
        return fmt.Errorf("failed to get device count: %v", ret)
    }
    
    for i := 0; i < count; i++ {
        device, ret := p.nvmllib.DeviceGetHandleByIndex(i)
        if ret != nvml.SUCCESS {
            p.logger.Error(nil, "Failed to get device handle", "index", i)
            continue
        }
        
        gpuInfo, err := p.getGpuInfo(device)
        if err != nil {
            p.logger.Error(err, "Failed to get GPU info", "index", i)
            continue
        }
        
        // Register GPU with initial healthy state
        gpu := &v1alpha1.Gpu{
            Name: gpuInfo.UUID,
            Spec: &v1alpha1.GpuSpec{
                Uuid:         gpuInfo.UUID,
                Index:        int32(i),
                ProductName:  gpuInfo.ProductName,
                Architecture: gpuInfo.Architecture,
                MemoryBytes:  gpuInfo.MemoryBytes,
                PciBusId:     gpuInfo.PCIBusID,
            },
            Status: &v1alpha1.GpuStatus{
                Phase: v1alpha1.GpuPhase_GPU_PHASE_READY,
                Conditions: []*v1alpha1.Condition{
                    {
                        Type:    "NVMLReady",
                        Status:  v1alpha1.ConditionStatus_CONDITION_STATUS_TRUE,
                        Reason:  "Initialized",
                        Message: "GPU enumerated via NVML",
                    },
                },
            },
        }
        
        p.cache.Set(gpu)
        p.logger.Info("Registered GPU", "uuid", gpuInfo.UUID, "name", gpuInfo.ProductName)
    }
    
    return nil
}
```

### XID Event Monitoring

Monitor for critical XID errors that indicate GPU health issues:

```go
func (p *NVMLProvider) StartHealthMonitoring(ctx context.Context) error {
    if ret := p.nvmllib.Init(); ret != nvml.SUCCESS {
        return fmt.Errorf("NVML init failed: %v", ret)
    }
    
    eventSet, ret := p.nvmllib.EventSetCreate()
    if ret != nvml.SUCCESS {
        p.nvmllib.Shutdown()
        return fmt.Errorf("failed to create event set: %v", ret)
    }
    p.eventSet = eventSet
    
    // Register for health-related events on all GPUs
    eventMask := uint64(
        nvml.EventTypeXidCriticalError |
        nvml.EventTypeDoubleBitEccError |
        nvml.EventTypeSingleBitEccError,
    )
    
    count, _ := p.nvmllib.DeviceGetCount()
    for i := 0; i < count; i++ {
        device, _ := p.nvmllib.DeviceGetHandleByIndex(i)
        supportedEvents, _ := device.GetSupportedEventTypes()
        device.RegisterEvents(eventMask & supportedEvents, p.eventSet)
    }
    
    p.ctx, p.cancel = context.WithCancel(ctx)
    p.wg.Add(1)
    go p.monitorEvents()
    
    return nil
}

func (p *NVMLProvider) monitorEvents() {
    defer p.wg.Done()
    defer p.nvmllib.Shutdown()
    defer p.eventSet.Free()
    
    for {
        select {
        case <-p.ctx.Done():
            return
        default:
            event, ret := p.eventSet.Wait(5000) // 5s timeout
            if ret == nvml.ERROR_TIMEOUT {
                continue
            }
            if ret != nvml.SUCCESS {
                if ret == nvml.ERROR_GPU_IS_LOST {
                    p.markAllUnhealthy("GPULost", "GPU is lost")
                }
                continue
            }
            
            p.handleEvent(event)
        }
    }
}

func (p *NVMLProvider) handleEvent(event nvml.EventData) {
    if event.EventType != nvml.EventTypeXidCriticalError {
        return
    }
    
    xid := event.EventData
    if p.isIgnoredXid(xid) {
        p.logger.V(2).Info("Ignoring non-critical XID", "xid", xid)
        return
    }
    
    uuid, _ := event.Device.GetUUID()
    
    // Update condition to unhealthy
    condition := &v1alpha1.Condition{
        Type:    "NVMLReady",
        Status:  v1alpha1.ConditionStatus_CONDITION_STATUS_FALSE,
        Reason:  "XidError",
        Message: fmt.Sprintf("Critical XID error %d detected", xid),
    }
    
    p.cache.UpdateCondition(uuid, condition)
    p.logger.Info("GPU marked unhealthy", "uuid", uuid, "xid", xid)
}

// XIDs that indicate application errors, not GPU hardware issues
var ignoredXids = map[uint64]bool{
    13:  true, // Graphics Engine Exception
    31:  true, // GPU memory page fault
    43:  true, // GPU stopped processing
    45:  true, // Preemptive cleanup
    68:  true, // Video processor exception
    109: true, // Context Switch Timeout
}

func (p *NVMLProvider) isIgnoredXid(xid uint64) bool {
    return ignoredXids[xid]
}
```

---

## Health Condition Model

### Condition-Based Health

Each provider owns specific condition types. Overall GPU health is determined by aggregating all conditions:

```protobuf
message Condition {
  // Type identifies the condition (e.g., "NVMLReady", "DCGMHealth")
  string type = 1;
  
  // Status: TRUE, FALSE, or UNKNOWN
  ConditionStatus status = 2;
  
  // Reason is a brief CamelCase reason for the condition
  string reason = 3;
  
  // Message provides human-readable details
  string message = 4;
  
  // LastTransitionTime when condition last changed
  google.protobuf.Timestamp last_transition_time = 5;
  
  // Source identifies who set this condition
  string source = 6;
}
```

### Standard Condition Types

| Type | Owner | Description |
|------|-------|-------------|
| `NVMLReady` | Built-in NVML Provider | Basic NVML health (XID errors, ECC) |
| `DCGMHealth` | DCGM-based provider | DCGM health checks |
| `NVSentinelHealth` | NVSentinel monitor | Advanced health analysis |
| `Ready` | Aggregated | Overall readiness (computed) |

### Health Aggregation

```go
// IsHealthy returns true if all conditions are healthy
func (g *Gpu) IsHealthy() bool {
    for _, c := range g.Status.Conditions {
        if c.Status != ConditionStatus_CONDITION_STATUS_TRUE {
            return false
        }
    }
    return true
}

// GetCondition returns a specific condition by type
func (g *Gpu) GetCondition(conditionType string) *Condition {
    for _, c := range g.Status.Conditions {
        if c.Type == conditionType {
            return c
        }
    }
    return nil
}
```

### Provider Priority

When multiple providers update the same GPU:

1. **Condition types are namespaced** - No conflicts between providers
2. **External providers can add conditions** - Supplements NVML data
3. **No overwriting** - Each provider manages only its own condition type

```
Example: GPU with multiple providers
═══════════════════════════════════

GPU: GPU-abc123
├── Condition: NVMLReady = True (source: nvml-provider)
├── Condition: DCGMHealth = True (source: dcgm-exporter)  
└── Condition: NVSentinelHealth = False (source: nvsentinel, reason: RowRemapPending)

Overall health: UNHEALTHY (one condition is False)
```

---

## Deployment Configuration

### RuntimeClass Pattern

The key insight from [dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) is using RuntimeClass to inject NVML without consuming GPU resources:

```yaml
# RuntimeClass (pre-requisite, typically installed by GPU Operator)
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: nvidia
handler: nvidia
```

### Helm Values Extension

```yaml
# values.yaml additions

# NVML provider configuration
nvml:
  # Enable built-in NVML provider (fallback when no external providers)
  enabled: true
  
  # Driver root path (for locating libnvidia-ml.so)
  driverRoot: /run/nvidia/driver
  
  # Additional XIDs to ignore (comma-separated)
  additionalIgnoredXids: ""
  
  # Health monitoring interval (seconds)
  healthCheckInterval: 5

# RuntimeClass for NVML access without GPU resource consumption
runtimeClassName: nvidia

# Environment for NVML access
env:
  - name: NVIDIA_VISIBLE_DEVICES
    value: "all"
  - name: NVIDIA_DRIVER_CAPABILITIES
    value: "utility"  # Only need nvidia-smi/NVML, not compute
```

### DaemonSet Changes

```yaml
# templates/daemonset.yaml additions

apiVersion: apps/v1
kind: DaemonSet
spec:
  template:
    spec:
      # RuntimeClass injects nvidia driver/NVML without resource requests
      {{- if .Values.runtimeClassName }}
      runtimeClassName: {{ .Values.runtimeClassName }}
      {{- end }}
      
      containers:
        - name: {{ .Chart.Name }}
          args:
            {{- if .Values.nvml.enabled }}
            - --enable-nvml-provider
            - --nvml-driver-root={{ .Values.nvml.driverRoot }}
            {{- if .Values.nvml.additionalIgnoredXids }}
            - --nvml-ignored-xids={{ .Values.nvml.additionalIgnoredXids }}
            {{- end }}
            {{- end }}
            # ... existing args ...
          
          env:
            {{- if .Values.nvml.enabled }}
            - name: NVIDIA_VISIBLE_DEVICES
              value: "all"
            - name: NVIDIA_DRIVER_CAPABILITIES
              value: "utility"
            {{- end }}
            # ... existing env ...
          
          # Note: NO nvidia.com/gpu resource requests!
          # RuntimeClass handles device injection
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 256Mi
```

### Graceful Degradation

The server must handle NVML unavailability gracefully:

```go
func (s *Server) Start(ctx context.Context) error {
    // ... existing startup ...
    
    if s.config.NVMLEnabled {
        nvmlProvider, err := nvml.NewNVMLProvider(s.cache, s.config.NVMLDriverRoot)
        if err != nil {
            // Log warning but continue - external providers can still work
            s.logger.Info("NVML provider disabled", "reason", err.Error())
        } else {
            if err := nvmlProvider.EnumerateDevices(); err != nil {
                s.logger.Info("NVML enumeration failed", "reason", err.Error())
            } else {
                if err := nvmlProvider.StartHealthMonitoring(ctx); err != nil {
                    s.logger.Info("NVML health monitoring disabled", "reason", err.Error())
                }
                s.nvmlProvider = nvmlProvider
            }
        }
    }
    
    // Server is ready even without NVML
    s.setReady(true)
    
    // ... rest of startup ...
}
```

---

## Implementation Plan

### New Files

```
pkg/deviceapiserver/
├── nvml/
│   ├── provider.go          # NVMLProvider implementation
│   ├── provider_test.go     # Unit tests
│   ├── enumerator.go        # Device enumeration logic
│   ├── health_monitor.go    # XID event monitoring
│   └── xid.go               # XID classification (ignored vs critical)
```

### Implementation Tasks

| ID | Task | Size | Dependencies |
|----|------|------|--------------|
| NV1 | Add `github.com/NVIDIA/go-nvml` dependency | S | - |
| NV2 | Implement `NVMLProvider` struct and lifecycle | M | NV1 |
| NV3 | Implement device enumeration | M | NV2 |
| NV4 | Implement XID event monitoring | M | NV2 |
| NV5 | Add condition-based health model to proto | M | - |
| NV6 | Integrate with server startup/shutdown | S | NV2-NV4 |
| NV7 | Add CLI flags and config options | S | NV6 |
| NV8 | Update Helm chart with RuntimeClass | M | NV7 |
| NV9 | Unit tests for NVML provider | L | NV2-NV4 |
| NV10 | Integration tests with mock NVML | L | NV9 |
| NV11 | Documentation | M | All |

### CLI Flags

```
--enable-nvml-provider     Enable built-in NVML health provider (default: false)
--nvml-driver-root         Root path for NVIDIA driver (default: /run/nvidia/driver)
--nvml-ignored-xids        Additional XIDs to ignore (comma-separated)
--nvml-health-interval     Health check interval in seconds (default: 5)
```

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| NVML unavailable (no driver) | Low | Medium | Graceful degradation, server starts without NVML |
| RuntimeClass not installed | Medium | Low | Clear documentation, helm chart validation |
| XID false positives | Medium | Low | Configurable ignored XIDs list |
| Resource leak (NVML handles) | Medium | Low | Proper cleanup in shutdown, defer patterns |
| Race between NVML and gRPC providers | Low | Medium | Condition-based model prevents conflicts |

---

## Alternatives Considered

### Alternative 1: Mandatory External Provider

**Description**: Require external provider deployment, no built-in NVML.

**Rejected because**:
- Increases deployment complexity
- Bootstrap problem remains
- No baseline health without provider

### Alternative 2: HostPath Volume for NVML

**Description**: Mount `/dev/nvidia*` and driver libraries via hostPath.

**Rejected because**:
- Security concerns with direct device access
- Complex volume configuration
- RuntimeClass is the standard pattern

### Alternative 3: GPU Resource Request with Shared Access

**Description**: Request `nvidia.com/gpu: 0` or fractional GPU.

**Rejected because**:
- Not supported by standard device plugin
- Would block actual GPU workloads on resource-constrained nodes

---

## References

1. [NVIDIA/k8s-dra-driver-gpu - device_health.go](https://github.com/NVIDIA/k8s-dra-driver-gpu/blob/main/cmd/gpu-kubelet-plugin/device_health.go)
2. [NVIDIA/k8s-dra-driver-gpu - nvlib.go](https://github.com/NVIDIA/k8s-dra-driver-gpu/blob/main/cmd/gpu-kubelet-plugin/nvlib.go)
3. [NVIDIA/dcgm-exporter - deployment](https://github.com/NVIDIA/dcgm-exporter/tree/main/deployment)
4. [NVIDIA/go-nvml](https://github.com/NVIDIA/go-nvml)
5. [NVIDIA XID Errors](https://docs.nvidia.com/deploy/xid-errors/index.html)
6. [Kubernetes RuntimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/)

---

*Document version: 1.0*  
*Last updated: 2026-01-21*
