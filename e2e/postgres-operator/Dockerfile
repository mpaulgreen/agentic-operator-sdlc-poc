# Build the manager binary using native cross-compilation
# --platform=$BUILDPLATFORM runs the build on the host architecture (e.g., arm64 on Mac)
# while GOARCH=${TARGETARCH} cross-compiles the binary for the target (e.g., amd64 for OpenShift)
FROM --platform=$BUILDPLATFORM golang:1.22 AS builder
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Runtime image — Red Hat UBI for OpenShift certification compliance
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

# Red Hat required labels for certification
LABEL name="postgres-operator" \
      vendor="Example" \
      version="0.1.0" \
      summary="PostgresCluster Operator" \
      description="Manages PostgresCluster resources on OpenShift" \
      io.k8s.display-name="PostgresCluster Operator" \
      io.k8s.description="Manages PostgresCluster resources on OpenShift" \
      io.openshift.tags="operator,database"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
