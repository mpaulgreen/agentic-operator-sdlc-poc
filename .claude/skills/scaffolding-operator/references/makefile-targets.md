# Makefile Targets Reference

The operator Makefile provides ~50 targets organized into categories. All targets use these configuration variables at the top.

## Configuration Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `VERSION` | `0.0.1` | Operator version for OLM bundle |
| `IMAGE_TAG_BASE` | `<domain>/<operator-name>` | Container image registry path |
| `IMG` | `controller:latest` | Full image reference for build/push |
| `OPERATOR_SDK_VERSION` | `v1.37.0` | Operator SDK version for bundle generation |
| `ENVTEST_K8S_VERSION` | `1.29.0` | Kubernetes version for envtest binaries |
| `CONTAINER_TOOL` | `podman` | Container build tool (podman/docker) |
| `KUSTOMIZE_VERSION` | `v5.3.0` | Kustomize binary version |
| `CONTROLLER_TOOLS_VERSION` | `v0.14.0` | controller-gen version |
| `ENVTEST_VERSION` | `release-0.17` | setup-envtest version |
| `GOLANGCI_LINT_VERSION` | `v1.57.2` | Linter version |
| `PLATFORMS` | `linux/arm64,linux/amd64,linux/s390x,linux/ppc64le` | Cross-compile targets |

## Development Targets

| Target | Command | Purpose |
|--------|---------|---------|
| `manifests` | `controller-gen rbac:roleName=manager-role crd webhook paths="./..."` | Generate CRD YAML, RBAC roles, webhook configs from Go markers |
| `generate` | `controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."` | Generate DeepCopy methods for API types |
| `fmt` | `go fmt ./...` | Format Go source |
| `vet` | `go vet ./...` | Static analysis |
| `test` | `go test $(shell go list ./... | grep -v /e2e) -coverprofile cover.out` | Unit + integration tests with envtest |
| `test-e2e` | `go test ./test/e2e/ -v -ginkgo.v` | E2E tests against real cluster |
| `lint` | `golangci-lint run` | Lint with golangci-lint |
| `lint-fix` | `golangci-lint run --fix` | Auto-fix lint issues |

## Build Targets

| Target | Purpose |
|--------|---------|
| `build` | `go build -o bin/manager cmd/main.go` |
| `run` | `go run ./cmd/main.go` (run locally against cluster) |
| `docker-build` | Build container image with `$(CONTAINER_TOOL)` |
| `docker-push` | Push image to registry |
| `docker-buildx` | Cross-platform build for all `$(PLATFORMS)` |
| `build-installer` | Kustomize build → `dist/install.yaml` (standalone installer) |

## Deployment Targets

| Target | Purpose |
|--------|---------|
| `install` | Install CRDs into cluster (`kustomize build config/crd | kubectl apply`) |
| `uninstall` | Remove CRDs from cluster |
| `deploy` | Deploy operator to cluster (`kustomize build config/default | kubectl apply`) |
| `undeploy` | Remove operator from cluster |

## OLM / Bundle Targets

| Target | Purpose |
|--------|---------|
| `bundle` | Generate OLM bundle (CSV + CRD + metadata) using operator-sdk |
| `bundle-build` | Build bundle container image |
| `bundle-push` | Push bundle image |
| `catalog-build` | Build OLM catalog index image using opm |
| `catalog-push` | Push catalog image |

## Tool Dependency Targets

Tools are downloaded to `$(LOCALBIN)` (default: `bin/`):

| Target | Tool |
|--------|------|
| `kustomize` | Kustomize binary |
| `controller-gen` | controller-gen for CRD/RBAC generation |
| `envtest` | setup-envtest for test environment |
| `golangci-lint` | Linter |
| `operator-sdk` | Operator SDK CLI (for bundle generation) |

Each tool target checks if the correct version exists before downloading.
