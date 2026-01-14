# client-gen

This is a customized version of the `v0.34.1` Kubernetes [client-gen](https://github.com/kubernetes/code-generator/tree/master/cmd/client-gen).

It generates a typed versioned Go **Clientset** for accessing API resources. Unlike the standard generator which defaults to REST/HTTP, this version creates clients that support **gRPC transport** for the NVIDIA Device API.

## Workflow

The generation process follows three main steps:

### 1. Tagging API Types

In your API definition files (e.g., `api/device/v1alpha1/types.go`), mark the types (e.g., `GPU`) that you want to generate clients for using `// +genclient`. If the resource is *not* namespaced, append `// +genclient:nonNamespaced`.

#### Supported Tags

* `// +genclient` - Generate default client verb functions (`create`, `update`, `delete`, `get`, `list`, `patch`, `watch`, and `updateStatus`).
* `// +genclient:nonNamespaced` - Generate verb functions without namespace parameters.
* `// +genclient:onlyVerbs=<verb>,<verb>` - Generate **only** the listed verbs.
* `// +genclient:skipVerbs=<verb>,<verb>` - Generate all default verbs **except** the listed ones.
* `// +genclient:noStatus` - Skip `updateStatus` verb even if the `.Status` struct field exists.
* `// +groupName=policy.authorization.k8s.io` – Overrides the API group name (defaults to the package name).
* `// +groupGoName=AuthorizationPolicy` – Sets a custom Golang identifier to de-conflict groups (defaults to the upper-case first segment of the group name).

### 2. Running the Generator

> [!NOTE]
> This binary is typically invoked via [kube_codegen.sh](../../README.md).

If running manually, use the following flags to link your API types to the Protobuf stubs:

```bash
client-gen \
  --output-dir "client" \
  --output-pkg "github.com/my-org/my-project/client" \
  --clientset-name "versioned" \
  --input-base "github.com/my-org/my-project/api" \
  --input "mygroup/v1alpha1" \
  --proto-base "github.com/my-org/my-project/api/gen/go"
```

The generator resolves packages by combining `--input-base` and `--input` (e.g., `github.com/my-org/my-project/api/mygroup/v1alpha1`).

### 3. Adding Expansion Methods

`client-gen` only generates standard CRUD methods. Add additional methods through the expansion interface by creating a file named `${TYPE}_expansion.go` in the generated typed directory, defining a `${TYPE}Expansion` interface, and implementing the methods.

The generator automatically detects this file and embeds the custom expansion interface into the generated client.

## Output Structure

The generator produces a tiered directory structure:
- **Clientset**: Found at `${output-dir}/${versioned-name}/${clientset-name}/`.
- **Typed Clients**: Found at `${output-dir}/${versioned-name}/${clientset-name}/typed/${group}/${version}/`.
- **Fake Clientset**: Found at `${output-dir}/${versioned-name}/${clientset-name}/fake/`.
- **Fake Typed Clients**: Found at `${output-dir}/${versioned-name}/${clientset-name}/typed/${group}/${version}/fake/`.

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| **`--proto-base`** | **Yes** | The base Go import path for the generated Protobuf stubs (e.g., `github.com/org/repo/api/gen/go`). |
| **`--input`** | **Yes** | Comma-separated list of groups/versions to generate (e.g., `device/v1alpha1,networking/v1`). |
| **`--input-base`** | **Yes** | Base import path for the API types (e.g., `github.com/org/repo/api`). |
| **`--output-pkg`** | **Yes** | Go package path for the generated files (e.g., `github.com/org/repo/client`). |
| **`--output-dir`** | **Yes** | Base directory for the output on disk (e.g., `./client`). |
| `--boilerplate` | No | Path to a header file (copyright/license) to prepend to generated files. Default: `hack/boilerplate.go.txt`. |
| `--clientset-name` | No | Name of the generated package/directory. Default: `clientset`. |
| `--versioned-name` | No | Name of the versioned clientset directory. Default: `versioned`. |
| `--plural-exceptions`| No | Comma-separated list of `Type:PluralizedType` overrides. |
| `--fake-clientset` | No | Generate a fake clientset that can be used in tests. Default: `true` |
| `--prefers-protobuf` | **N/A** | **Removed** This generator assumes Protobuf/gRPC support is always enabled. |
| `--apply-configuration-package` | **N/A** | **Not Implemented** |
