# AGENTS.md - Persistent Context for AI Assistants

> **Last Updated:** 2026-02-04
> **Active Branch:** `feat/cloud-native-healthevents` ✅ READY
> **Base Commit:** `105cd6c` (Use cuda image from NVCR to avoid rate limits (#792))
> **Head Commit:** `4457249` (feat: implement cloud-native GPU health event management)
> **New PR:** https://github.com/NVIDIA/NVSentinel/pull/795 (DRAFT)
> **Old PR:** https://github.com/NVIDIA/NVSentinel/pull/794 (superseded - incorrectly based)
> **Status:** ✅ Cherry-pick complete - 22 commits applied cleanly onto main

---

## Current Task: Rebase Cloud-Native Storage onto Main

### Problem
The `feat/cloud-native-storage` branch was created from commit `59578f3`, which is on the
`device-api-server` lineage **after** commit `d6c5c46` that deleted all NVSentinel core code.

The branch is missing:
- `commons/`, `data-models/`, `health-monitors/`, `labeler/`, `lint/`
- `remediations/`, `reports/`, `scalers/`, `sentinel/`, `services/`
- `CONTRIBUTING.md`, `.coderabbit.yaml`, `RELEASE.md`, `ROADMAP.md`
- Most GitHub Actions workflows

### Solution
Cherry-pick our commits onto a fresh branch from `origin/main`.

### Commits to Cherry-Pick (in order)
```
7fc1f70 chore - Add small set of GitHub checks and copilot rules (#691)
a07681f feat: introduce k8s-idiomatic Go SDK for Device API (#692)
6a94259 chore: update documentation (#693)
8ad7b7a api: add ProviderService proto for device-api-server
b907fb1 feat: add device-api-server with NVML fallback provider
95cea2e fix(security): bind gRPC TCP listener to localhost by default
cf88679 docs(provider): clarify Heartbeat RPC is reserved for future use
ff88ab8 feat(consumer): populate ListMeta.ResourceVersion in ListGpus response
e37ed8c docs: document module structure and naming conventions
bd1b3ad fix: align module paths to github.com/nvidia/nvsentinel
cf1a193 refactor: consolidate to unified GpuService with standard CRUD methods
f7de28b fix(build): use Go 1.25 for container builds
fd4c919 fix(nvml-provider): parse command-line flags before returning config
cdbcfc3 fix(helm): remove invalid --provider-address flag from server args
18bb07c fix(helm): correct sidecar test values for cluster deployment
de7751e feat(demo): improve cross-platform builds and idempotency
28ef6be fix(demo): remove unreliable metrics check from verify_gpu_registration
cc2fcce fix(demo): use correct container name 'nvsentinel' instead of 'device-api-server'
18946e1 fix(ci): update protoc version to v33.4 to match generated files
a231cf3 docs: add hybrid device-apiserver design for PR #718 + #720 merge
59578f3 chore: add .worktrees/ and temp docs to .gitignore
3260812 feat: implement cloud-native GPU health event management
```

### Worktree Locations
- **Old (broken):** `/Users/eduardoa/src/github/nvidia/NVSentinel/.worktrees/cloud-native-storage` (to be removed)
- **New (active):** `/Users/eduardoa/src/github/nvidia/NVSentinel/.worktrees/cloud-native-healthevents` ✅

---

## Feature: Cloud-Native GPU Health Event Management

### Architecture
```
┌─────────────────┐    gRPC    ┌──────────────────┐
│ HealthProvider  │ ─────────▶│ Device-API-Server│
│   (DaemonSet)   │            │                  │
└─────────────────┘            └────────┬─────────┘
        │                               │
        │ NVML                          │ CRDPublisher
        ▼                               ▼
    ┌───────┐                   ┌───────────────┐
    │  GPU  │                   │  HealthEvent  │
    └───────┘                   │     (CRD)     │
                                └───────┬───────┘
                                        │
              ┌─────────────────────────┼─────────────────────────┐
              │                         │                         │
              ▼                         ▼                         ▼
    ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
    │ QuarantineCtrl   │───▶│   DrainCtrl      │───▶│ RemediationCtrl  │
    │  (cordon node)   │    │  (evict pods)    │    │ (reboot/reset)   │
    └──────────────────┘    └──────────────────┘    └──────────────────┘
```

### Phase Progression
```
New → Quarantined → Drained → Remediated → Resolved
```

### Key Files Created
```
api/nvsentinel/v1alpha1/
├── doc.go
├── groupversion_info.go
├── healthevent_types.go
└── zz_generated.deepcopy.go

pkg/controllers/healthevents/
├── conditions.go
├── drain_controller.go
├── drain_controller_test.go
├── metrics.go
├── quarantine_controller.go
├── quarantine_controller_test.go
├── remediation_controller.go
├── remediation_controller_test.go
├── ttl_controller.go
└── ttl_controller_test.go

pkg/deviceapiserver/crdpublisher/
├── metrics.go
├── publisher.go
└── publisher_test.go

pkg/healthprovider/
├── nvml_source.go
├── nvml_source_stub.go
├── provider.go
└── provider_test.go

cmd/controller-test/main.go
cmd/health-provider/main.go
cmd/health-provider/main_stub.go

deployments/helm/nvsentinel/crds/nvsentinel.nvidia.com_healthevents.yaml
deployments/helm/nvsentinel/templates/policy-configmap.yaml
deployments/helm/health-provider/
```

### Integration Test Results (2026-02-04)
- **Cluster:** AWS EKS with GPUs
- **Kubeconfig:** `/Users/eduardoa/.kube/config-aws-gpu`
- **Result:** SUCCESS - Full phase progression in 2 seconds
- **Node tested:** `ip-10-0-0-236`

### Important Fixes Applied
1. **Predicate removal:** Controllers don't use `WithEventFilter` for phase filtering
   (status subresource updates don't reliably trigger watch events)
2. **Empty phase handling:** `QuarantineController` treats `phase=""` as `phase=New`
3. **Field index:** Added `spec.nodeName` index for Pods in controller-test main.go

---

## Commands Reference

### Create new worktree from main
```bash
cd /Users/eduardoa/src/github/nvidia/NVSentinel
git fetch origin main
git worktree add .worktrees/cloud-native-healthevents origin/main -b feat/cloud-native-healthevents
```

### Cherry-pick all commits
```bash
cd .worktrees/cloud-native-healthevents
git cherry-pick 7fc1f70^..3260812
```

### Run integration test
```bash
export KUBECONFIG=/Users/eduardoa/.kube/config-aws-gpu
./bin/controller-test --health-probe-bind-address=:18081 --metrics-bind-address=:18080 -v=2
```

### Create test HealthEvent
```yaml
apiVersion: nvsentinel.nvidia.com/v1alpha1
kind: HealthEvent
metadata:
  name: test-full-flow
spec:
  source: integration-test
  nodeName: <NODE_NAME>
  componentClass: GPU
  checkName: xid-error-check
  isFatal: true
  recommendedAction: RESTART_VM
  detectedAt: "2026-02-04T16:00:00Z"
```

---

## Related PRs and Issues
- PR #794: Draft PR for cloud-native storage (needs rebasing)
- PR #718, #720: Original device-api-server proposals being enhanced
- Design doc: `docs/plans/2026-02-04-hybrid-device-apiserver-design.md`
