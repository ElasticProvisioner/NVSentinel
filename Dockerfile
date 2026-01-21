# Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build stage (without NVML - for NVML support use Dockerfile.nvml)
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG GIT_TREE_STATE=dirty
ARG BUILD_DATE

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files first for caching
COPY go.mod go.sum ./
COPY api/go.mod api/go.sum ./api/

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build without NVML (CGO disabled, pure Go)
# For NVML support, use: docker build -f Dockerfile.nvml .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
        -X github.com/nvidia/nvsentinel/pkg/version.Version=${VERSION} \
        -X github.com/nvidia/nvsentinel/pkg/version.GitCommit=${GIT_COMMIT} \
        -X github.com/nvidia/nvsentinel/pkg/version.GitTreeState=${GIT_TREE_STATE} \
        -X github.com/nvidia/nvsentinel/pkg/version.BuildDate=${BUILD_DATE}" \
    -o /device-api-server \
    ./cmd/device-api-server

# Runtime stage - use Alpine for better compatibility
FROM alpine:3.21

LABEL org.opencontainers.image.source="https://github.com/nvidia/nvsentinel"
LABEL org.opencontainers.image.description="Device API Server - Node-local GPU device state cache"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# Add ca-certificates for HTTPS (nobody user already exists as uid 65534)
RUN apk add --no-cache ca-certificates

WORKDIR /

COPY --from=builder --chmod=755 /device-api-server /device-api-server

USER 65534:65534

ENTRYPOINT ["/device-api-server"]
