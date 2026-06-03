# Build Stage
FROM golang:1.26.4 AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy module files first to leverage Docker caching
COPY kustomize-mutating-webhook/go.mod kustomize-mutating-webhook/go.sum ./
RUN go mod download

# Copy entire source
COPY kustomize-mutating-webhook/ .

# Build the application targeting cmd/webhook/main.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
  go build -trimpath -ldflags "-s -w" -o webhook ./cmd/webhook/main.go

# Deploy Stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /
VOLUME [ "/etc/config" ]

# Copy the compiled binary from the builder stage
COPY --from=builder /app/webhook /webhook

# Set the entrypoint
ENTRYPOINT ["/webhook"]
