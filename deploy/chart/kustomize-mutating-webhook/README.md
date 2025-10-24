# kustomize-mutating-webhook

![Version: 0.7.0](https://img.shields.io/badge/Version-0.7.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.5.0](https://img.shields.io/badge/AppVersion-0.5.0-informational?style=flat-square)

A Helm chart for the Kustomize Mutating Webhook chart

## Introduction

This Helm chart deploys a mutating webhook for FluxCD Kustomization resources. It extends the functionality of postBuild substitutions beyond the scope of a single namespace, allowing the use of global configuration variables stored in a central namespace.

## Prerequisites

- Kubernetes >=1.19.0-0
- Helm v3+
- FluxCD installed in the cluster
- cert-manager for managing TLS certificates

## Installing the Chart

To install the chart with the release name `fluxcd-mutating-webhook`:

```console
$ helm repo add fluxcd-mutating-webhook https://xunholy.github.io/fluxcd-mutating-webhook/
$ helm install fluxcd-mutating-webhook fluxcd-mutating-webhook/kustomize-mutating-webhook
```

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```console
$ helm delete my-release
```

## Configuration

The following table lists the configurable parameters of the kustomize-mutating-webhook chart and their default values.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| additionalLabels | object | `{}` | Additional labels to add to all resources |
| affinity | object | `{}` | Affinity rules for pod assignment |
| annotations | object | `{}` | Annotations to add to all resources |
| certManager.CASClusterIssuer.enabled | bool | `false` | Enable AWS Private CA or Google CAS cluster issuer |
| certManager.CASClusterIssuer.group | string | `"awspca.cert-manager.io"` | API group for the CAS issuer (awspca.cert-manager.io or cas-issuer.jetstack.io) |
| certManager.CASClusterIssuer.kind | string | `"AWSPCAClusterIssuer"` | Kind of CAS issuer (AWSPCAClusterIssuer or GoogleCASClusterIssuer) |
| certManager.CASClusterIssuer.name | string | `"casissuer-name"` | Name of the CAS cluster issuer |
| certManager.certificateDuration | string | `"2160h"` | Certificate duration (90 days default) |
| certManager.certificateRenewBefore | string | `"360h"` | Certificate renewal threshold (15 days before expiry) |
| certManager.enabled | bool | `true` | Enable cert-manager integration for TLS certificate management |
| configMaps | list | `[{"create":false,"data":{},"name":"cluster-config","optional":false}]` | ConfigMaps to mount into the webhook container for substitution variables |
| env.LOG_LEVEL | string | `"info"` | Log level (debug, info, warn, error, fatal, panic) |
| env.RATE_LIMIT | string | `"100"` | Rate limit for webhook requests per second |
| fullnameOverride | string | `""` | Override the full name of the release |
| image.pullPolicy | string | `"Always"` | Image pull policy |
| image.repository | string | `"ghcr.io/xunholy/kustomize-mutating-webhook"` | Container image repository |
| image.tag | string | `"main-fee1c33"` | Image tag (overrides the image tag whose default is the chart appVersion) |
| imagePullSecrets | list | `[]` | Secrets for pulling images from private registries |
| nameOverride | string | `""` | Override the name of the chart |
| networkpolicy.create | bool | `true` | Create a NetworkPolicy to restrict traffic to the webhook |
| podAnnotations | object | `{}` | Annotations to add to the pod |
| podDisruptionBudget.enabled | bool | `false` | Enable pod disruption budget (recommended for replicas >= 3) |
| podDisruptionBudget.minAvailable | int | `2` | Minimum number of available pods during disruptions |
| podSecurityContext.runAsGroup | int | `1000` | Group ID to run the container as |
| podSecurityContext.runAsNonRoot | bool | `true` | Run container as non-root user |
| podSecurityContext.runAsUser | int | `1000` | User ID to run the container as |
| replicas | int | `1` | Number of webhook pod replicas |
| resources.limits.cpu | string | `"500m"` | CPU resource limits |
| resources.limits.memory | string | `"256Mi"` | Memory resource limits |
| resources.requests.cpu | string | `"100m"` | CPU resource requests |
| resources.requests.memory | string | `"128Mi"` | Memory resource requests |
| secrets | list | `[]` | Secrets to mount into the webhook container for substitution variables |
| securityContext.allowPrivilegeEscalation | bool | `false` | Prevent privilege escalation |
| securityContext.capabilities.drop | list | `["ALL"]` | Drop all capabilities |
| securityContext.readOnlyRootFilesystem | bool | `true` | Mount root filesystem as read-only |
| service.headless | bool | `true` | Create a headless service (no cluster IP) |
| service.port | int | `8443` | Service port for webhook server |
| service.type | string | `"ClusterIP"` | Kubernetes service type |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.name | string | `""` | The name of the service account to use. If not set and create is true, a name is generated using the fullname template |
| tolerations | list | `[]` | Tolerations for pod assignment |
| webhook.failurePolicy | string | `"Fail"` | Failure policy for the mutating webhook (Fail or Ignore) |
| webhook.namespaceSelector.matchExpressions | list | `[{"key":"kubernetes.io/metadata.name","operator":"NotIn","values":["flux-system"]}]` | Match expressions to select namespaces where the webhook should apply |
| webhook.timeoutSeconds | int | `30` | Timeout in seconds for the webhook |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```console
$ helm install my-release myrepo/kustomize-mutating-webhook --set replicas=3
```

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart. For example,

```console
$ helm install my-release myrepo/kustomize-mutating-webhook -f values.yaml
```

## TLS Configuration

This chart uses cert-manager to manage TLS certificates for the webhook. Ensure that cert-manager is properly set up in your cluster and that you've configured the `certManager` values in this chart to use the correct issuer.

## Customizing the Mutating Webhook

The mutating webhook is configured to intercept CREATE and UPDATE operations on Kustomization resources. You can customize this behavior by modifying the `webhook` values in the chart.

## Monitoring and Logging

The webhook server includes `/health` and `/ready` endpoints for monitoring. Configure your preferred monitoring solution to track these endpoints.

Logs are output to stdout and can be collected using your cluster's logging solution.

## Troubleshooting

If you encounter issues with the webhook, check the following:

1. Ensure the webhook pod is running:
   ```
   kubectl get pods -l app.kubernetes.io/name=kustomize-mutating-webhook
   ```

2. Check the logs of the webhook pod:
   ```
   kubectl logs -l app.kubernetes.io/name=kustomize-mutating-webhook
   ```

3. Verify the MutatingWebhookConfiguration:
   ```
   kubectl get mutatingwebhookconfigurations
   ```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## Requirements

Kubernetes: `>=1.19.0-0`

---

For more information on using Helm, refer to the [Helm Documentation](https://helm.sh/docs/).
