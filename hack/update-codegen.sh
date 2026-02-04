#!/usr/bin/env bash

#  Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#       http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"

CODEGEN_ROOT="./code-generator"
export KUBE_CODEGEN_ROOT="${CODEGEN_ROOT}"

source "${CODEGEN_ROOT}/kube_codegen.sh"


# Hack: prevent circular dependency
#
# Examples and integration tests depend on codegen artifacts, temporarily hiding 
# to allow codegen to execute successfully; trap ensures they are always restored.
cleanup() {
  if [ -d "_examples" ] || [ -d "_test" ]; then
    echo "Restoring examples and test directories..."
    mv _examples examples 2>/dev/null || true
    mv _test test 2>/dev/null || true
  fi
}

trap cleanup EXIT SIGINT SIGTERM


if [ -d "examples" ] || [ -d "test" ]; then
  echo "Temporarily hiding examples and test directories..."
  mv examples _examples 2>/dev/null || true
  mv test _test 2>/dev/null || true
fi

###

kube::codegen::gen_proto_bindings \
  --output-dir "${REPO_ROOT}/internal/generated" \
  --proto-root "proto" \
  "${REPO_ROOT}/api"

go mod tidy

kube::codegen::gen_helpers \
  --boilerplate "hack/boilerplate.go.txt" \
  "./api"

kube::codegen::gen_client \
  --proto-base "github.com/nvidia/nvsentinel/internal/generated" \
  --output-dir "${REPO_ROOT}/pkg/client-go" \
  --output-pkg "github.com/nvidia/nvsentinel/pkg/client-go" \
  --boilerplate "hack/boilerplate.go.txt" \
  --clientset-name "client" \
  --versioned-name "versioned" \
  --with-watch \
  --listers-name "listers" \
  --informers-name "informers" \
  "${REPO_ROOT}/api"
