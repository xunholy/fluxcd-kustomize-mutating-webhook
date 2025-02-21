
## Below are the steps to manually generate a certificate for the scenario where cert-manger is not available

```
#  Disable cert-manager in values.yam
certManager:
  enabled: false

# Generate the CA certificate and key:
cfssl gencert -initca deploy/certs/csr.json | cfssljson -bare deploy/certs/ca

# Generate TLS certificates for the webhook:
cfssl gencert \
-ca=deploy/certs/ca.pem \
-ca-key=deploy/certs/ca-key.pem \
-config=deploy/certs/config.json \
-hostname="kustomize-mutating-webhook.flux-system.svc" \
-profile=client \
deploy/certs/kustomize-mutating-webhook-csr.json | cfssljson -bare deploy/certs/kustomize-mutating-webhook

# Create k8s secret with certificate
kubectl create secret tls kustomize-mutating-webhook-tls --key=deploy/certs/kustomize-mutating-webhook-key.pem --cert=deploy/certs/kustomize-mutating-webhook.pem -n flux-system --dry-run=client -o yaml | kubectl apply -f -

# Deploy helm chart
helm install kustomize-mutating-webhook deploy/chart/fluxcd-mutating-webhook
```
