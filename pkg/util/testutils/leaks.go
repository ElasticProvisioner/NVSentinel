//  Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package testutils

import (
	"testing"

	"go.uber.org/goleak"
	"k8s.io/klog/v2"
)

// IgnoreOptions returns a standard set of goleak.Options.
func IgnoreOptions() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreTopFunction("k8s.io/klog/v2.(*flushDaemon).run.func1"),
		goleak.IgnoreTopFunction("google.golang.org/grpc.(*pickerWrapper).pick"),
		goleak.IgnoreTopFunction("google.golang.org/grpc.(*ClientConn).WaitForStateChange"),
		goleak.IgnoreTopFunction("google.golang.org/grpc.(*addrConn).resetTransportAndUnlock"),
		goleak.IgnoreTopFunction("google.golang.org/grpc/internal/grpcsync.(*CallbackSerializer).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*watchGRPCStream).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*watcher).Watch"),
		goleak.IgnoreTopFunction("k8s.io/apiserver/pkg/storage/etcd3.(*compactor).runWatchLoop"),
		goleak.IgnoreTopFunction("k8s.io/apiserver/pkg/storage/etcd3.(*compactor).runCompactLoop"),
		goleak.IgnoreTopFunction("k8s.io/apimachinery/pkg/util/wait.BackoffUntilWithContext"),
		goleak.IgnoreTopFunction("k8s.io/apimachinery/pkg/util/wait.loopConditionUntilContext"),
		goleak.IgnoreTopFunction("context.(*cancelCtx).propagateCancel.func2"),
	}
}

// VerifyNone marks the given TestingT as failed if any extra goroutines are
// found. Wraps goleak.VerifyNone with standard ignores and klog cleanup.
func VerifyNone(t *testing.T) {
	t.Cleanup(func() {
		klog.StopFlushDaemon()
		goleak.VerifyNone(t, IgnoreOptions()...)
	})
}

// VerifyTestMain can be used in a TestMain function for package tests to
// verify that there were no goroutine leaks. Wraps goleak.VerifyTestMain
// with standard ignores and klog cleanup.
func VerifyTestMain(m *testing.M) {
	goleak.VerifyTestMain(m, append(IgnoreOptions(), goleak.Cleanup(func(int) {
		klog.StopFlushDaemon()
	}))...)
}
