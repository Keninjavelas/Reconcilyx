# Build the embedded dashboard
# Pin major Node LTS release; supply immutable @sha256 digest via CI build args for air-gapped builds.
FROM --platform=$BUILDPLATFORM node:20.19.0-alpine AS ui-builder
WORKDIR /workspace/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.26.11-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our layer as much
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY . .
# Overlay the dashboard built in the UI stage so go:embed always has assets
COPY --from=ui-builder /workspace/ui/dist ./ui/dist

# Build reproducible static stripped binary (CGO_ENABLED=0, -ldflags="-s -w")
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o manager cmd/main.go

# Use distroless nonroot as minimal runtime base image (zero shell, UID/GID 65532)
# Production releases verify base image digests with cosign + Trivy in .github/workflows/release.yml
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="reconcilyx" \
      org.opencontainers.image.description="Kubernetes FinOps, GreenOps carbon, and right-sizing operator" \
      org.opencontainers.image.license="Apache-2.0"
ENV HOME=/tmp
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
