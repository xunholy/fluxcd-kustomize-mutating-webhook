template-manifests:
	# Generate the raw manifests
	helm template kustomize-mutating-webhook deploy/chart/fluxcd-mutating-webhook -n flux-system > raw_manifests.yaml

populate-ca-bundle: template-manifests
	# Populate the CA Bundle inside the Webhook config manifest
	CA_PEM_B64=$$(openssl base64 -A < deploy/certs/ca.pem); \
	sed -e 's@CA_PEM_B64@'"$$CA_PEM_B64"'@g' < raw_manifests.yaml > raw_manifests_deploy.yaml

generate-certs:
	# Generate CA Certificate and Key
	cfssl gencert -initca deploy/certs/ca-csr.json | cfssljson -bare deploy/certs/ca

	# Generate Webhook Certificate and Key
	cfssl gencert \
	-ca=deploy/certs/ca.pem \
	-ca-key=deploy/certs/ca-key.pem \
	-config=deploy/certs/config.json \
	-hostname="kustomize-mutating-webhook.flux-system.svc" \
	-profile=client \
	deploy/certs/kustomize-mutating-webhook-csr.json | cfssljson -bare deploy/certs/kustomize-mutating-webhook

verify-certs: generate-certs
	# Verify Webhook Certificate Against the CA
	openssl verify -CAfile deploy/certs/ca.pem deploy/certs/kustomize-mutating-webhook.pem

template: generate-certs populate-ca-bundle
	# Template the manifests
	cat raw_manifests_deploy.yaml

deploy: generate-certs populate-ca-bundle
	# Deploy the manifests
	kubectl create secret tls kustomize-mutating-webhook-tls --key=deploy/certs/kustomize-mutating-webhook-key.pem --cert=deploy/certs/kustomize-mutating-webhook.pem -n flux-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f raw_manifests_deploy.yaml -n flux-system

clean:
	# Clean up the generated files and k8s resources
	kubectl delete -f raw_manifests_deploy.yaml --ignore-not-found=true	-n flux-system
	kubectl delete secret tls kustomize-mutating-webhook-tls --ignore-not-found=true -n flux-system
	rm deploy/certs/*.pem || true
	rm deploy/certs/*.csr || true
	rm raw_manifests.yaml || true
	rm raw_manifests_deploy.yaml || true
