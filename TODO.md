# TODO

---

### General

- [ ] **OSRB**
- [x] standardize project layout
- [x] trim down / consolidate docs
- [ ] development.md
- [ ] **Go type as single-source-of-truth**
- [ ] internal superset type
- [ ] api validation
- [ ] discovery api
    - *removes the need for users to provide a RESTMapper to controller-runtime manager*

---

### Server

- [ ] **design doc**
- [ ] **auth interceptor**
- [ ] validation interceptor
- [ ] **audit logs**
- [x] health checks
- [x] metrics server
- [x] admin interface
- [x] version metric
- [x] version endpoint
- [ ] datastore
  - [ ] ?additional Kine compaction control?
  - [ ] ?additional SQLite compaction control?
  - [ ] ?export db size metric?
  - [ ] ?server-side cache?
- [ ] service-gen
  - [ ] ?fake server?
- [ ] deployment
  - [ ] image
  - [ ] helm chart
  - [ ] publish
  - [ ] docs
- [x] unit tests
- [ ] integration tests
- [ ] performance tests

---

### Client

- [ ] client-gen
  - [ ] discovery api
  - [ ] **implement updateStatus template**
  - [ ] **implement patch template**
  - [ ] ?aggregated clientset w/ standard k8s clientset?
- [ ] integration tests
- [ ] ?add version to request header?

---

### NVSentinel

- [ ] ?design doc?
- [ ] integration
  - [ ] ?new module?

---

### Device Plugin

- [ ] ?design doc?
- [ ] integration

### DRA Driver

- [ ] **design doc**
- [ ] integration

### GPU Operator
 
- [ ] integration

---

---

# Scratch

---

```bash 
$ sudo mkdir -p /var/run/nvidia-device-apiserver
$ sudo chmod 755 /var/lib/nvidia-device-apiserver 
```

```bash
$ sudo ./bin/device-apiserver --version
$ sudo ./bin/device-apiserver --version=raw

$ sudo ./bin/device-apiserver --help

$ sudo ./bin/device-apiserver --v=2 2>&1 | grep -v "etcd-client"

## Admin: Channelz, Reflection, and Health
$ sudo grpcurl -plaintext localhost:50051 list

$ sudo grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

## CREATE
$ sudo grpcurl -plaintext \
  -unix -d '{
    "gpu": {
      "metadata": {
        "name": "gpu-7",
        "namespace": "default"
      },
      "spec": {
        "uuid": "GPU-a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6"
      }
    }
  }' \
  unix:/var/run/nvidia-device-api/device-api.sock \
  nvidia.nvsentinel.v1alpha1.GpuService/CreateGpu

## GET
$ sudo grpcurl -plaintext \
  -unix -d '{"name": "gpu-7", "namespace": "default"}' \
  unix:/var/run/nvidia-device-api/device-api.sock \
  nvidia.nvsentinel.v1alpha1.GpuService/GetGpu

## UPDATE
$ sudo grpcurl -plaintext \
  -unix -d '{
    "gpu": {
      "metadata": {
        "name": "gpu-7",
        "namespace": "default",
        "resource_version": "<rv>"
      },
      "spec": {
        "uuid": "GPU-a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6"
      },
      "status": {
        "conditions": [
          {
            "type": "Ready",
            "status": "True",
            "reason": "DriverReady",
            "message": "Driver is posting a Ready status."
          }
        ]
      }
    }
  }' \
  unix:/var/run/nvidia-device-api/device-api.sock \
  nvidia.nvsentinel.v1alpha1.GpuService/UpdateGpu

## GET
$ sudo grpcurl -plaintext \
  -unix -d '{"name": "gpu-7", "namespace": "default"}' \
  unix:/var/run/nvidia-device-api/device-api.sock \
  nvidia.nvsentinel.v1alpha1.GpuService/GetGpu

## LIST
$ sudo grpcurl -plaintext \
  -unix -d '{"namespace": "", "opts": {"resource_version": "0"}}' \
  unix:/var/run/nvidia-device-api/device-api.sock \
  nvidia.nvsentinel.v1alpha1.GpuService/ListGpus
  ```

---
