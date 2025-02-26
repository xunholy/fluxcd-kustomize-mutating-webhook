
## Below are the steps to manually generate a certificate for the scenario where cert-manger is not available

**Note:** Have cfssl and openssl installed! 

```
#  Disable cert-manager in values.yam
certManager:
  enabled: false

# Check webhook raw manifests with base64 encoded CA bundle inside the webhook config
make template

# Deploy webhook raw manifests with base64 encoded CA bundle inside the webhook config
make deploy
```
