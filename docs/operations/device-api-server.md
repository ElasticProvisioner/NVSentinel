# Device API Server - Operations Guide

This guide covers deployment, configuration, monitoring, and troubleshooting of the Device API Server.

## Architecture Overview

The Device API Server is a pure Go gRPC server with no hardware dependencies.
GPU enumeration and health monitoring is provided by external providers (sidecars).

```
┌─────────────────────────────────────────────────────────────┐
│                        GPU Node                              │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                Device API Server (DaemonSet)            ││
│  │  ┌─────────────────────────────────────────────────┐   ││
│  │  │               GpuService (unified)              │   ││
│  │  │  Read:  GetGpu, ListGpus, WatchGpus             │   ││
│  │  │  Write: CreateGpu, UpdateGpuStatus, DeleteGpu   │   ││
│  │  └────────────────────┬────────────────────────────┘   ││
│  │                       │                                 ││
│  │                       ▼                                 ││
│  │  ┌─────────────────────────────────────────────────────┐││
│  │  │                  GPU Cache (RWMutex)                │││
│  │  │  - Read-blocking during writes                      │││
│  │  │  - Watch event broadcasting                         │││
│  │  └─────────────────────────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────┘│
│                                                              │
│  Providers (gRPC clients):                                   │
│  - nvml-provider sidecar (GPU enumeration, XID monitoring)   │
│  - Custom providers (CreateGpu, UpdateGpuStatus)             │
│                                                              │
│  Consumers (gRPC clients):                                   │
│  - Device plugins (GetGpu, ListGpus, WatchGpus)              │
│  - DRA drivers (GetGpu, ListGpus, WatchGpus)                 │
└─────────────────────────────────────────────────────────────┘
```

## Deployment

### Prerequisites

- Kubernetes 1.25+
- Helm 3.0+
- GPU nodes with label `nvidia.com/gpu.present=true`
- (Optional) Prometheus Operator for monitoring

### Installation

**Basic Installation**:

```bash
helm install device-api-server ./deployments/helm/device-api-server \
  --namespace device-api --create-namespace
```

**With Prometheus Monitoring**:

```bash
helm install device-api-server ./deployments/helm/device-api-server \
  --namespace device-api --create-namespace \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.prometheusRule.enabled=true
```

### Verify Installation

```bash
# Check DaemonSet status
kubectl get daemonset -n device-api

# Check pods are running on GPU nodes
kubectl get pods -n device-api -o wide

# Check logs
kubectl logs -n device-api -l app.kubernetes.io/name=device-api-server
```

---

## Configuration

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--grpc-address` | `127.0.0.1:50051` | TCP address for gRPC server (localhost only by default) |
| `--unix-socket` | `/var/run/device-api/device.sock` | Unix socket path |
| `--health-port` | `8081` | HTTP port for health endpoints |
| `--metrics-port` | `9090` | HTTP port for Prometheus metrics |
| `--shutdown-timeout` | `30` | Graceful shutdown timeout (seconds) |
| `--shutdown-delay` | `5` | Pre-shutdown delay (seconds) |
| `--log-format` | `json` | Log format: `text` or `json` |
| `-v` | `0` | Log verbosity level |

### Helm Values

See [values.yaml](../../deployments/helm/device-api-server/values.yaml) for the complete reference.

Key configuration sections:

```yaml
# Server configuration
server:
  grpcAddress: "127.0.0.1:50051"  # localhost only by default for security
  unixSocket: /var/run/device-api/device.sock
  healthPort: 8081
  metricsPort: 9090

# Node scheduling
nodeSelector:
  nvidia.com/gpu.present: "true"

# Resources
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 200m
    memory: 256Mi
```

---

## GPU Providers

The Device API Server is a pure Go gRPC server with no hardware dependencies.
GPU enumeration and health monitoring is provided by external providers that connect
as gRPC clients:

- **nvml-provider sidecar** - Recommended NVML-based provider for GPU enumeration and XID monitoring
- **Custom providers** - Any gRPC client can register GPUs via `CreateGpu` and update health via `UpdateGpuStatus`

See the [nvml-provider demo](../../demos/nvml-sidecar-demo.sh) for an example sidecar deployment.

---

## Monitoring

### Health Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe - server is running |
| `/readyz` | 8081 | Readiness probe - server is accepting traffic |
| `/metrics` | 9090 | Prometheus metrics |

### Prometheus Metrics

**Server Metrics**:

| Metric | Type | Description |
|--------|------|-------------|
| `device_api_server_info` | Gauge | Server information (version, go_version) |

**Cache Metrics**:

| Metric | Type | Description |
|--------|------|-------------|
| `device_api_server_cache_gpus_total` | Gauge | Total GPUs in cache |
| `device_api_server_cache_gpus_healthy` | Gauge | Healthy GPUs |
| `device_api_server_cache_gpus_unhealthy` | Gauge | Unhealthy GPUs |
| `device_api_server_cache_gpus_unknown` | Gauge | GPUs with unknown status |
| `device_api_server_cache_updates_total` | Counter | Cache update operations |
| `device_api_server_cache_resource_version` | Gauge | Current cache version |

**Watch Metrics**:

| Metric | Type | Description |
|--------|------|-------------|
| `device_api_server_watch_streams_active` | Gauge | Active watch streams |
| `device_api_server_watch_events_total` | Counter | Watch events sent |

### Alerting Rules

When `metrics.prometheusRule.enabled=true`, the following alerts are created:

| Alert | Severity | Condition |
|-------|----------|-----------|
| `DeviceAPIServerDown` | Critical | Server unreachable for 5m |
| `DeviceAPIServerHighLatency` | Warning | P99 latency > 500ms |
| `DeviceAPIServerHighErrorRate` | Warning | Error rate > 10% |
| `DeviceAPIServerUnhealthyGPUs` | Warning | Unhealthy GPUs > 0 |
| `DeviceAPIServerNoGPUs` | Warning | No GPUs for 10m |
| `DeviceAPIServerHighMemory` | Warning | Memory > 512MB |

### Grafana Dashboard

Example PromQL queries for dashboards:

```promql
# GPU health overview
device_api_server_cache_gpus_healthy / device_api_server_cache_gpus_total * 100

# Watch stream activity
rate(device_api_server_watch_events_total[5m])

# Cache update rate
rate(device_api_server_cache_updates_total[5m])
```

---

## Troubleshooting

### Pod Not Scheduling

**Symptom**: DaemonSet shows 0/N pods ready

**Check**:

```bash
# Verify node labels
kubectl get nodes --show-labels | grep gpu

# Check DaemonSet events
kubectl describe daemonset -n device-api device-api-server
```

**Solution**: Ensure nodes have `nvidia.com/gpu.present=true` label or override `nodeSelector`.

### Permission Denied on Unix Socket

**Symptom**: Clients cannot connect to Unix socket

**Check**:

```bash
# Check socket permissions on node
ls -la /var/run/device-api/
```

**Solution**: Verify `securityContext` allows socket creation, or adjust `runAsUser`.

### GPUs Not Appearing

**Symptom**: `ListGpus` returns empty

**Check**:

```bash
# Check for GPU enumeration errors
kubectl logs -n device-api <pod> | grep -i error

# Check if provider sidecar is running
kubectl get pods -n device-api -o wide
```

**Solutions**:
1. Deploy the nvml-provider sidecar: see [nvml-provider demo](../../demos/nvml-sidecar-demo.sh)
2. Deploy an external health provider
3. Verify the provider can connect to the Device API Server

### High Memory Usage

**Symptom**: Pod OOMKilled or memory alerts firing

**Check**:

```bash
# Check current memory usage
kubectl top pods -n device-api

# Check watch stream count
curl -s http://<pod-ip>:9090/metrics | grep watch_streams
```

**Solutions**:
1. Increase memory limits
2. Investigate clients creating excessive watch streams
3. Check for memory leaks in logs

### Watch Stream Disconnections

**Symptom**: Consumers report frequent reconnections

**Check**:

```bash
# Check network policy
kubectl get networkpolicy -n device-api

# Check for errors in logs
kubectl logs -n device-api <pod> | grep -i "stream\|watch"
```

**Solutions**:
1. Ensure network policies allow intra-node traffic
2. Check client timeout settings
3. Verify server is not overloaded

---

## Graceful Shutdown

The server implements graceful shutdown:

1. **PreStop Hook**: Sleeps for `shutdownDelay` seconds
2. **Signal Handling**: Catches SIGTERM/SIGINT
3. **Drain Period**: Stops accepting new connections
4. **In-Flight Completion**: Waits for active requests (up to `shutdownTimeout`)
5. **Resource Cleanup**: Closes connections

**Timeline**:

```
SIGTERM → [shutdownDelay] → Stop listeners → [shutdownTimeout] → Force close
```

Configure in Helm:

```yaml
server:
  shutdownTimeout: 30  # Max wait for in-flight requests
  shutdownDelay: 5     # Pre-shutdown delay for endpoint propagation
```

---

## Security Considerations

### Pod Security

Default security context (non-root, restricted):

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65534
  runAsGroup: 65534
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```

### Network Security

> **Warning**: The gRPC API is unauthenticated. Take care when exposing beyond localhost.

- gRPC over TCP is **plaintext and unauthenticated** by default and is intended **only for node-local access**.
- The TCP listener binds to `127.0.0.1:50051` (localhost) by default. This prevents network exposure.
- Prefer using a Unix domain socket for all local clients and providers; avoid exposing the TCP gRPC listener to the broader cluster network.
- In multi-tenant or partially untrusted clusters, strongly recommend **disabling the TCP listener** (set `--grpc-address=""`) or using a restricted Unix socket combined with Kubernetes `NetworkPolicy` to limit access to the Device API Server pod.
- If TCP gRPC must be exposed beyond the node (e.g., `--grpc-address=:50051`), terminate it behind TLS/mTLS or an equivalent authenticated tunnel (for example, a sidecar proxy or ingress with client auth) and enforce a least-privilege `NetworkPolicy`.

### Service Account

- `automountServiceAccountToken: false` by default
- No Kubernetes API access required

---

## See Also

- [API Reference](../api/device-api-server.md)
- [Design Document](../design/device-api-server.md)
- [Helm Chart README](../../deployments/helm/device-api-server/README.md)
- [NVML Sidecar Demo](../../demos/nvml-sidecar-demo.sh)
