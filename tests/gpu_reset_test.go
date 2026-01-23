//go:build arm64_group
// +build arm64_group

// Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"tests/helpers"

	"github.com/nvidia/nvsentinel/commons/pkg/statemanager"
)

func TestGPUReset(t *testing.T) {
	feature := features.New("TestGPUReset").
		WithLabel("suite", "gpu-reset")

	var immediateEvictionPods []string
	var immediateEvictionPodsWithImpactedGPU []string
	var initialDCGMPod string
	feature.Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		workloadNamespace := "immediate-test"

		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		ctx = helpers.ApplyQuarantineConfig(ctx, t, c, "data/basic-matching-configmap.yaml")
		ctx = helpers.ApplyNodeDrainerConfig(ctx, t, c, "data/nd-all-modes.yaml")

		// Use a real (non-KWOK) node for smoke test to validate actual container execution
		nodeName, err := helpers.GetRealNodeName(ctx, client)
		assert.NoError(t, err, "failed to get real node")
		t.Logf("Selected real node for GPU reset test: %s", nodeName)

		err = helpers.CreateNamespace(ctx, client, workloadNamespace)
		assert.NoError(t, err, "failed to create workloads namespace")

		immediateEvictionPods = helpers.CreatePodsFromTemplate(ctx, t, client, "data/busybox-pods.yaml", nodeName, workloadNamespace)
		immediateEvictionPodsWithImpactedGPU = helpers.CreatePodsFromTemplate(ctx, t, client, "data/busybox-pod-with-devices.yaml", nodeName, "immediate-test")

		helpers.WaitForPodsRunning(ctx, t, client, workloadNamespace, append(immediateEvictionPods,
			immediateEvictionPodsWithImpactedGPU...))

		initialDCGMPod = getDCGMPodOnNode(ctx, t, client, nodeName)
		t.Logf("Initial DCGM pod is : %s", initialDCGMPod)

		ctx = context.WithValue(ctx, keyNodeName, nodeName)
		ctx = context.WithValue(ctx, keyNamespace, workloadNamespace)

		return ctx
	})

	nodeLabelSequenceObserved := make(chan bool)
	feature.Assess("Can start node label watcher", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		nodeName := ctx.Value(keyNodeName).(string)
		t.Logf("Starting label sequence watcher for node %s", nodeName)
		desiredNVSentinelStateNodeLabels := []string{
			string(statemanager.QuarantinedLabelValue),
			string(statemanager.DrainingLabelValue),
			string(statemanager.DrainSucceededLabelValue),
			string(statemanager.RemediatingLabelValue),
			string(statemanager.RemediationSucceededLabelValue),
		}
		err = helpers.StartNodeLabelWatcher(ctx, t, client, nodeName, desiredNVSentinelStateNodeLabels, true, nodeLabelSequenceObserved)
		assert.NoError(t, err, "failed to start node label watcher")

		// Sleep to ensure Kubernetes watch is fully established before triggering state changes
		// This prevents missing early label transitions due to watch startup latency
		t.Log("Waiting for watch to establish connection...")
		time.Sleep(2 * time.Second)
		t.Log("Watch established, ready for health event")

		return ctx
	})

	feature.Assess("Can send fatal health event", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		nodeName := ctx.Value(keyNodeName).(string)

		err := helpers.SendHealthEventsToNodes([]string{nodeName}, "data/fatal-health-event-component-reset.json")
		assert.NoError(t, err, "failed to send health event")

		return ctx
	})

	feature.Assess("Node is cordoned", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		nodeName := ctx.Value(keyNodeName).(string)

		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		t.Logf("Waiting for node %s to be cordoned", nodeName)
		helpers.WaitForNodesCordonState(ctx, t, client, []string{nodeName}, true)

		node, err := helpers.GetNodeByName(ctx, client, nodeName)
		assert.NoError(t, err, "failed to get node after cordoning")

		assert.Equal(t, "NVSentinel", node.Labels["cordon-by"])
		assert.Equal(t, "Basic-Match-Rule", node.Labels["cordon-reason"])

		var nodeCondition *v1.NodeCondition
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == "GpuXidError" {
				nodeCondition = &node.Status.Conditions[i]
				break
			}
		}
		assert.NotNil(t, nodeCondition, "node condition GpuXidError not found")

		assert.Equal(t, "GpuXidError", string(nodeCondition.Type))
		assert.Equal(t, "True", string(nodeCondition.Status))
		assert.Equal(t, "GpuXidErrorIsNotHealthy", nodeCondition.Reason)
		assert.Equal(t, "ErrorCode:119 GPU:0 GPU_UUID:GPU-455d8f70-2051-db6c-0430-ffc457bff834 PCI:0000:03:00 XID error occurred Recommended Action=COMPONENT_RESET;", nodeCondition.Message)

		return ctx
	})

	feature.Assess("Wait for pod leveraging GPU to be drained", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		namespaceName := ctx.Value(keyNamespace).(string)

		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		helpers.WaitForPodsDeleted(ctx, t, client, namespaceName, immediateEvictionPodsWithImpactedGPU)

		helpers.AssertPodsNeverDeleted(ctx, t, client, namespaceName, immediateEvictionPods)

		return ctx
	})

	feature.Assess("GPUReset CR is created and completes", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		nodeName := ctx.Value(keyNodeName).(string)
		namespaceName := ctx.Value(keyNamespace).(string)

		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		// ensure gpu-operator pods are torn down as part of GPUReset custom resources
		helpers.WaitForPodsDeleted(ctx, t, client, namespaceName, []string{initialDCGMPod})

		gpuReset := helpers.WaitForCR(ctx, t, client, nodeName, helpers.GPUResetGVK)
		status, found, err := unstructured.NestedMap(gpuReset.Object, "status")
		if err != nil || !found {
			assert.Fail(t, "failed to find status field in CR", gpuReset.GetName(), err)
		}
		conditions, found, err := unstructured.NestedSlice(status, "conditions")
		if err != nil || !found {
			assert.Fail(t, "failed to find status conditions field in CR", gpuReset.GetName(), err)
		}
		var foundCompleteCondition bool
		for _, c := range conditions {
			condMap := c.(map[string]interface{})

			if condMap["type"].(string) == "Complete" {
				foundCompleteCondition = true
				assert.Equal(t, "GPUResetSucceeded", condMap["reason"].(string))
				assert.Equal(t, "True", condMap["status"].(string))
			}
		}
		assert.True(t, foundCompleteCondition, "Did not find Complete condition on CR", gpuReset.GetName())

		// ensure gpu-operator pods are restored
		newDCGMPod := getDCGMPodOnNode(ctx, t, client, nodeName)
		t.Logf("Restored DCGM pod is : %s", newDCGMPod)

		err = helpers.DeleteCR(ctx, client, gpuReset)
		assert.NoError(t, err, "failed to delete GPUReset CR")

		return ctx
	})

	feature.Assess("Can send healthy event", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		nodeName := ctx.Value(keyNodeName).(string)

		err := helpers.SendHealthEventsToNodes([]string{nodeName}, "data/healthy-event-component-reset.json")
		assert.NoError(t, err, "failed to send health event")

		return ctx
	})

	feature.Assess("Node is uncordoned", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		nodeName := ctx.Value(keyNodeName).(string)

		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		t.Logf("Waiting for node %s to be uncordoned", nodeName)
		helpers.WaitForNodesCordonState(ctx, t, client, []string{nodeName}, false)

		// Wait for node condition to be updated to healthy
		t.Logf("Waiting for node %s condition to become healthy", nodeName)
		helpers.WaitForNodeConditionWithCheckName(ctx, t, client, nodeName, "GpuXidError", "", "GpuXidErrorIsHealthy", v1.ConditionFalse)

		node, err := helpers.GetNodeByName(ctx, client, nodeName)
		assert.NoError(t, err, "failed to get node after uncordoning")

		assert.Equal(t, "NVSentinel", node.Labels["uncordon-by"])

		var nodeCondition *v1.NodeCondition
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == "GpuXidError" {
				nodeCondition = &node.Status.Conditions[i]
				break
			}
		}
		assert.NotNil(t, nodeCondition, "node condition GpuXidError not found")

		assert.Equal(t, "GpuXidError", string(nodeCondition.Type))
		assert.Equal(t, "False", string(nodeCondition.Status))
		assert.Equal(t, "GpuXidErrorIsHealthy", nodeCondition.Reason)
		assert.Equal(t, "No Health Failures", nodeCondition.Message)

		return ctx
	})

	feature.Assess("Confirm pods not leveraging GPU not drained", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		namespaceName := ctx.Value(keyNamespace).(string)

		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		helpers.AssertPodsNeverDeleted(ctx, t, client, namespaceName, immediateEvictionPods)
		helpers.DeletePodsByNames(ctx, t, client, namespaceName, immediateEvictionPods)

		return ctx
	})

	feature.Assess("Observed NVSentinel expected state label changes", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		select {
		case success := <-nodeLabelSequenceObserved:
			assert.True(t, success)
		default:
			assert.Fail(t, "did not observe expected label changes for nvsentinel-state")
		}
		return ctx
	})

	feature.Teardown(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		client, err := c.NewClient()
		assert.NoError(t, err, "failed to create kubernetes client")

		namespaceName := ctx.Value(keyNamespace).(string)
		err = helpers.DeleteNamespace(ctx, t, client, namespaceName)
		assert.NoError(t, err, "failed to delete workloads namespace")

		helpers.RestoreQuarantineConfig(ctx, t, c)
		helpers.RestoreNodeDrainerConfig(ctx, t, c)

		return ctx
	})

	testEnv.Test(t, feature.Feature())
}

func getDCGMPodOnNode(ctx context.Context, t *testing.T, client klient.Client, nodeName string) string {
	var initialDCGMPod string
	pods, err := helpers.GetPodsOnNode(ctx, client.Resources(), nodeName)
	assert.NoError(t, err, "failed to get pods on node %s", nodeName)
	for _, pod := range pods {
		if strings.Contains(pod.Name, "nvidia-dcgm") {
			initialDCGMPod = pod.Name
		}
	}
	if len(initialDCGMPod) == 0 {
		assert.FailNow(t, "failed to find nvidia-dcgm pod on node %s", nodeName)
	}
	return initialDCGMPod
}
