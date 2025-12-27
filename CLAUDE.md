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
# - AUTO_UPDATE_KUSTOMIZATIONS: trigger updates on config changes (default: true)
# - AUTO_UPDATE_EXCLUDE_NAMESPACES: comma-separated list of namespaces to exclude (default: flux-system)
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
- Recursively watches all subdirectories (required for Kubernetes ConfigMap/Secret mounts)
- Reacts to Write, Create, Remove, and Rename events (handles Kubernetes atomic updates via symlink swapping)
- Uses 100ms debounce to handle rapid successive changes
- Preserves old config on reload failure (logs errors but doesn't crash webhook)
- Thread-safe reload leveraging existing `AppConfig.Mu` RWMutex
- Optionally triggers Kustomization updates after successful reload (when auto-update is enabled)

**Kustomization Updater** (`kustomize-mutating-webhook/pkg/utils/kustomization_updater.go`)
- `KustomizationUpdater`: Optional component for triggering updates on existing Kustomizations
- Lists all Kustomizations across all namespaces (except excluded ones)
- Annotates each Kustomization with timestamp to trigger UPDATE events
- Causes webhook to re-inject latest config values into existing Kustomizations
- Only active when `AUTO_UPDATE_KUSTOMIZATIONS=true`
- Respects namespace exclusions (default: flux-system)

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

### Server-Side Apply (SSA) Requirement

**IMPORTANT**: This webhook requires FluxCD v2+ which uses **Server-Side Apply (SSA)** by default. SSA is critical for the webhook to work correctly with FluxCD's GitOps workflow.

#### How SSA Enables Webhook + FluxCD Coexistence

With Server-Side Apply:
1. **FluxCD manages only fields defined in Git**
   - If your Git repository doesn't define `spec.postBuild.substitute`, FluxCD won't manage that field
   - FluxCD only updates the fields it declares ownership of

2. **Webhook adds substitute values**
   - When FluxCD reconciles a Kustomization from Git, it sends an UPDATE request to the API server
   - The webhook intercepts this UPDATE and injects all ConfigMap/Secret values into `spec.postBuild.substitute`
   - These webhook-injected values persist across FluxCD reconciliations

3. **Both coexist without conflict**
   - FluxCD owns the Kustomization structure (path, interval, sourceRef, etc.)
   - Webhook owns the substitute values
   - Neither overwrites the other's fields

#### Recommended Git Configuration

**Option 1: Don't define substitute in Git** (Recommended)
```yaml
# In Git: clusters/production/apps/myapp.yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: myapp
spec:
  interval: 10m
  path: ./apps/myapp
  sourceRef:
    kind: GitRepository
    name: flux-system
  # No postBuild section - webhook handles everything
```

**Option 2: Define some values in Git, webhook adds more**
```yaml
# In Git
spec:
  postBuild:
    substitute:
      APP_ENV: production  # Git-managed value
      # Webhook will add: CLUSTER_ID, CLUSTER_DOMAIN, secrets, etc.
```

#### Verification

To verify SSA is working correctly:
```bash
# 1. Create or update a Kustomization
flux reconcile kustomization myapp --with-source

# 2. Check that webhook-injected values persist
kubectl get kustomization -n myapp-ns myapp -o jsonpath='{.spec.postBuild.substitute}' | jq
```

If values disappear after reconciliation, verify you're using FluxCD v2+ with SSA enabled.

### Auto-Update on Config Changes (Optional)

The webhook can optionally trigger updates on all existing Kustomizations when ConfigMaps or Secrets change, ensuring immediate propagation of new config values without waiting for FluxCD's natural reconciliation cycle.

#### Configuration

```bash
# Auto-update is enabled by default (default: true)
# To disable:
AUTO_UPDATE_KUSTOMIZATIONS=false

# Exclude namespaces from auto-update (default: flux-system)
# Comma-separated list
AUTO_UPDATE_EXCLUDE_NAMESPACES=flux-system,kube-system
```

#### How It Works

When enabled:
1. ConfigMap or Secret changes are detected by the ConfigWatcher
2. Configuration is reloaded successfully
3. All Kustomizations (except excluded namespaces) are annotated with `webhook.kustomize-mutating-webhook/config-reload=<timestamp>`
4. Annotation triggers UPDATE events on each Kustomization
5. Webhook intercepts each UPDATE and injects the latest config values
6. Kustomizations now have the updated values

#### Behavior

- **With auto-update** (default): New config values are immediately propagated to all existing Kustomizations
- **Without auto-update**: New config values are injected when Kustomizations are naturally created/updated by FluxCD

#### Example

```bash
# Update a ConfigMap
kubectl patch configmap cluster-config -n flux-system \
  --type merge -p '{"data":{"NEW_KEY":"new-value"}}'

# With auto-update enabled:
# - ConfigWatcher detects change within ~1 second
# - All Kustomizations (except flux-system) are annotated
# - Webhook injects NEW_KEY into all Kustomizations
# - Changes visible immediately

# Without auto-update:
# - ConfigWatcher still reloads config within ~1 second
# - Webhook has the new value in memory
# - Kustomizations get NEW_KEY when FluxCD next reconciles them (default: 10 minutes)
```

#### When to Use Auto-Update

**Use auto-update when:**
- You need immediate propagation of config changes (e.g., rotating credentials)
- You have many Kustomizations and don't want to wait for reconciliation
- You want consistent config across all resources immediately

**Don't use auto-update when:**
- You prefer GitOps-only changes (wait for FluxCD reconciliation)
- You have very large clusters (many Kustomizations) where batch updates might cause API server load
- You want to control rollout of config changes manually

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
