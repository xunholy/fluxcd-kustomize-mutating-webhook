# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Kubernetes mutating webhook for FluxCD Kustomization resources. It dynamically injects substitution variables from centralized ConfigMaps/Secrets into Kustomization resources at creation/update time, enabling global configuration management across multiple namespaces without requiring postBuild substitutions to be scoped to a single namespace.

## Development Commands

### Building
```bash
# Build the webhook binary
cd kustomize-mutating-webhook
go build -o webhook ./cmd/webhook

# Build with Docker (from repo root)
docker build -t kustomize-mutating-webhook .
```

### Testing
```bash
# Run all tests with verbose output
cd kustomize-mutating-webhook
go test -v ./...

# Run benchmarks with memory stats
go test -bench=. -benchmem

# Run tests for a specific package
go test -v ./internal/handlers
go test -v ./pkg/utils
```

### Running Locally
```bash
# Run the webhook (requires valid TLS certs and config)
./webhook

# Environment variables can be set:
# - LOG_LEVEL: debug, info, warn, error, fatal, panic (default: info)
# - SERVER_ADDRESS: server address (default: :8443)
# - CERT_FILE: TLS cert path (default: /etc/webhook/certs/tls.crt)
# - KEY_FILE: TLS key path (default: /etc/webhook/certs/tls.key)
# - CONFIG_DIR: config directory path (default: /etc/config)
# - RATE_LIMIT: requests per second (default: 100)
```

### Kubernetes Deployment

**Using Helm (OCI Registry)**
```bash
# Install from OCI registry (replace $OWNER with repository owner, e.g., xunholy)
helm install kustomize-mutating-webhook \
  oci://ghcr.io/$OWNER/charts/kustomize-mutating-webhook \
  --version 0.6.0 \
  --namespace flux-system
```

**Using Helm (Traditional Repository)**
```bash
# Add the Helm repository (replace $OWNER with repository owner, e.g., xunholy)
helm repo add kustomize-mutating-webhook https://$OWNER.github.io/fluxcd-kustomize-mutating-webhook
helm repo update

# Install the chart
helm install kustomize-mutating-webhook \
  kustomize-mutating-webhook/kustomize-mutating-webhook \
  --version 0.6.0 \
  --namespace flux-system
```

**Using Static Manifests**
```bash
# Deploy using static manifests
kubectl apply -k deploy/static

# Verify deployment
kubectl get pods --selector=app=kustomize-mutating-webhook -n flux-system

# View logs
kubectl logs --selector=app=kustomize-mutating-webhook -n flux-system
```

## Architecture

### Core Components

**Entry Point** (`kustomize-mutating-webhook/cmd/webhook/main.go`)
- Loads and validates configuration from environment variables
- Initializes logger with configurable log level
- Reads configuration from mounted ConfigMaps and/or Secrets in `/etc/config`
- Creates and starts the HTTP server with TLS
- Starts certificate watcher in a goroutine to hot-reload TLS certificates
- Starts config watcher in a goroutine to hot-reload ConfigMap and Secret changes
- Handles graceful shutdown on SIGINT/SIGTERM

**Configuration Management** (`kustomize-mutating-webhook/internal/config/`)
- `Config` struct holds all server configuration
- Environment-based config with sensible defaults
- Validation ensures required fields are present
- Logger initialization with zerolog

**Webhook Server** (`kustomize-mutating-webhook/internal/webhook/`)
- `server.go`: Sets up chi router with middleware (rate limiting, logging, recovery)
- Exposes endpoints: `/mutate` (main webhook), `/health`, `/ready`, `/metrics`
- TLS configuration uses certificate watcher for dynamic cert reloading
- `certwatcher.go`: Watches certificate directory using fsnotify, reloads on file changes

**Mutation Handler** (`kustomize-mutating-webhook/internal/handlers/mutate.go`)
- Receives AdmissionReview requests from Kubernetes API server
- Filters for Kustomization resources only
- Skips delete operations and resources with deletion timestamps
- Creates JSON patches to inject `/spec/postBuild/substitute` values
- Patches are created for all keys in the global `AppConfig.Config`
- Uses JSON Pointer escaping for keys with special characters

**Configuration Reader** (`kustomize-mutating-webhook/pkg/utils/utils.go`)
- `ReadConfigDirectory()`: Recursively walks config directory, reads all non-hidden files
- Stores key-value pairs in thread-safe `AppConfig` (global singleton with RWMutex)
- Keys are filenames, values are file contents
- Used by mutation handler to inject values into Kustomizations

**Configuration Watcher** (`kustomize-mutating-webhook/pkg/utils/configwatcher.go`)
- `ConfigWatcher`: Monitors config directory for file changes using fsnotify
- Automatically reloads configuration when ConfigMaps or Secrets are updated by Kubernetes
- Supports any combination of ConfigMaps and Secrets mounted to the config directory
- Reacts to Write, Create, Remove, and Rename events (handles Kubernetes atomic updates via symlink swapping)
- Uses 100ms debounce to handle rapid successive changes
- Preserves old config on reload failure (logs errors but doesn't crash webhook)
- Thread-safe reload leveraging existing `AppConfig.Mu` RWMutex

**Metrics** (`kustomize-mutating-webhook/internal/metrics/`)
- Prometheus metrics for monitoring webhook performance
- Tracks: total requests, mutation count, error count, request duration, rate limited requests

### Control Flow

1. **Startup**: Load config → Initialize logger → Read config directory → Create config watcher → Create server → Start cert watcher → Start config watcher → Start HTTPS server
2. **Mutation Request**: Receive AdmissionReview → Validate it's a Kustomization → Create JSON patches for substitute values → Return patched AdmissionReview
3. **Certificate Reload**: fsnotify detects cert file change → Reload certificate → New connections use updated cert
4. **Configuration Reload**: fsnotify detects config directory change → Sleep 100ms (debounce) → Reload config directory → Update AppConfig atomically
5. **Shutdown**: SIGINT/SIGTERM → Stop cert watcher → Stop config watcher → Graceful server shutdown with 30s timeout

### Key Design Decisions

- **Global Config Singleton**: `utils.AppConfig` is a global variable with mutex-protected access, allowing all handlers to read injected values
- **JSON Patch Strategy**: Adds `/spec/postBuild` and `/spec/postBuild/substitute` if missing, then adds individual key-value patches
- **Certificate Watching**: Enables cert rotation without pod restart (important for cert-manager renewals)
- **Config Hot-Reload**: ConfigMap/Secret changes propagate automatically within ~1 second without pod restart
- **Rate Limiting**: Token bucket algorithm (default 100 req/s) prevents webhook overload
- **Conditional Logging**: Request logging only enabled at debug level to reduce noise

### Important Configuration Notes

- The webhook expects ConfigMaps and/or Secrets to be mounted at `/etc/config` (configurable via `CONFIG_DIR`)
- By default, looks for a ConfigMap named `cluster-config`, but any mounted volumes work (ConfigMaps, Secrets, or both)
- All files in the config directory (and subdirectories) become available as substitute variables
- Kubernetes mounts ConfigMap and Secret keys as files (filename = key, content = value)
- **Hot-Reload**: Configuration changes are automatically detected and reloaded when Kubernetes updates mounted ConfigMaps or Secrets
  - Supports any combination: ConfigMaps only, Secrets only, or mixed ConfigMaps and Secrets
  - Changes propagate within ~1 second
  - Reload failures preserve the old configuration
  - Monitor reload events via logs or Prometheus metrics
  - Example: Update a Secret key and watch it reload without pod restart

### FluxCD Integration

The webhook intercepts FluxCD Kustomization resources before they're stored in etcd. When FluxCD's kustomize-controller reconciles a Kustomization, it uses the `spec.postBuild.substitute` values that were injected by this webhook to perform variable substitution in the rendered manifests.

## Release Process

This project uses [release-please](https://github.com/googleapis/release-please) for automated semantic versioning and release management with **multi-package support**.

### How It Works

1. **Conventional Commits**: Use conventional commit format for all commits:
   - `feat:` - New features (triggers minor version bump)
   - `fix:` - Bug fixes (triggers patch version bump)
   - `chore:` - Maintenance tasks (included in changelog)
   - Add `!` after type or `BREAKING CHANGE:` in footer for major version bumps

2. **Release PR Creation**: When commits are merged to `main`, release-please automatically:
   - Creates/updates **one grouped release PR** for both packages
   - Updates versions in `.release-please-manifest.json`
   - Updates `Chart.yaml` with both `version` and `appVersion` fields (via Helm release type)

3. **Release Creation**: When the release PR is merged, release-please creates **two separate releases**:
   - **App Release**: Tagged as `app-v1.2.3` - triggers Docker image build
   - **Helm Chart Release**: Tagged as `helm-chart-v1.2.3` - triggers Helm chart publish
   - Both versions stay synchronized (e.g., both become v1.2.3)

4. **Automated Builds**: Each release triggers its corresponding workflow:
   - **Docker Images** (`build-docker.yaml`) - Triggered by `app-v*` releases:
     - Builds multi-arch Docker images (amd64, arm64)
     - Tags images with clean semver: `v1.2.3`, `v1.2`, `v1`, `latest`
     - Signs images with Cosign
     - Pushes to `ghcr.io/$OWNER/kustomize-mutating-webhook`
   - **Helm Chart** (`helm-release.yaml`) - Triggered by `helm-chart-v*` releases:
     - Packages the Helm chart with the updated version
     - Pushes OCI chart to `ghcr.io/$OWNER/charts/kustomize-mutating-webhook`
     - Signs OCI chart with Cosign
   - **PR Builds** (`pr-build.yaml`) - Triggered by pull requests:
     - Runs tests to validate PR changes
     - Builds Docker image for linux/amd64 (fast builds for testing)
     - Pushes test images tagged as `pr-{number}` and `pr-{number}-{sha}`
     - Posts comment on PR with image details and usage instructions
     - Images are unsigned (test images only)

### Helm Chart Documentation

When making changes to the Helm chart in a PR, the `helm-docs.yaml` workflow will:
- Generate documentation from `values.yaml`
- Validate that the README is up to date
- Fail the PR if documentation needs updating

To generate Helm docs locally:
```bash
cd deploy/chart/kustomize-mutating-webhook
helm-docs
```

### Creating a Release

```bash
# 1. Make changes with conventional commits
git commit -m "feat: add new feature"
git commit -m "fix: resolve bug in handler"

# 2. Push to main (or merge PR to main)
git push origin main

# 3. Release-please creates a PR - review and merge it
# 4. Release is created automatically with tag
# 5. Docker images and Helm chart are built and pushed automatically
```

### Forcing a Release Without Changes

If you need to create a release without any code changes (e.g., to republish artifacts or create an initial release), you have two options:

**Option 1: Manually trigger release-please (Recommended)**
1. Go to **Actions** → **Release Please**
2. Click **Run workflow**
3. This will create/update the release PR based on current commits

**Option 2: Create an empty commit**
```bash
# Create an empty chore commit to trigger release-please
git commit --allow-empty -m "chore: trigger release"
git push origin main
```

Then merge the resulting release PR to create the release.

### Current Version

The current version is tracked in `.release-please-manifest.json`. The Helm chart version and appVersion in `deploy/chart/kustomize-mutating-webhook/Chart.yaml` are automatically updated by release-please.

### Testing Pull Requests

The `pr-build.yaml` workflow automatically builds and pushes test images for all pull requests:

1. **Automatic Build**: When you create or update a PR, a Docker image is built and pushed
2. **Image Tags**:
   - `ghcr.io/$OWNER/kustomize-mutating-webhook:pr-{number}` - Updated on each push to PR
   - `ghcr.io/$OWNER/kustomize-mutating-webhook:pr-{number}-{sha}` - Immutable per commit
3. **PR Comment**: A comment is automatically posted/updated with:
   - Image tags and digest
   - Instructions for testing the image with Helm or kubectl
   - Commit SHA reference
4. **Fast Builds**: Only builds for `linux/amd64` to reduce CI time (full multi-arch builds happen on release)

**Testing a PR image:**
```bash
# Pull and test locally
docker pull ghcr.io/$OWNER/kustomize-mutating-webhook:pr-123

# Deploy to test cluster with Helm
helm upgrade --install kustomize-mutating-webhook \
  oci://ghcr.io/$OWNER/charts/kustomize-mutating-webhook \
  --namespace flux-system \
  --set image.tag=pr-123

# Or with kubectl
kubectl set image deployment/kustomize-mutating-webhook \
  kustomize-mutating-webhook=ghcr.io/$OWNER/kustomize-mutating-webhook:pr-123 \
  -n flux-system
```

### Manual Re-triggering

Both the Docker build and Helm chart workflows can be manually triggered:

**Re-build Docker images for latest app release:**
1. Go to **Actions** → **Build, Test, and Push Docker Images**
2. Click **Run workflow**
3. Leave tag empty to use latest `app-v*` release, or specify a tag like `v1.2.3`
4. Note: Automatically finds and uses the latest `app-v*` release tag

**Re-publish Helm chart for latest release:**
1. Go to **Actions** → **Publish Helm Chart**
2. Click **Run workflow**
3. This will use the version in `Chart.yaml` from main branch

This is useful when you need to rebuild artifacts without creating a new release (e.g., after fixing registry issues or to update signatures).

### Version Synchronization

The Helm chart's `appVersion` field automatically tracks the Go app version. When release-please creates releases:
- The Helm release type updates both `Chart.yaml` `version` and `appVersion` fields
- Both releases get the same version number (e.g., 0.6.0)
- Tag names differ (`app-v0.6.0` vs `helm-chart-v0.6.0`) but versions match
- This ensures the Helm chart always references the correct app version

### Repository Settings Required

For release-please to work, ensure the following repository settings are configured:

1. Go to **Settings** → **Actions** → **General**
2. Under **Workflow permissions**, select **"Read and write permissions"**
3. Check **"Allow GitHub Actions to create and approve pull requests"**
