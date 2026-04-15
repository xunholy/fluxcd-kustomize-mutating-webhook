# Changelog

## [0.11.0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/compare/kustomize-mutating-webhook-v0.10.0...kustomize-mutating-webhook-v0.11.0) (2026-04-15)


### Features

* validate config keys against Flux substitute regex ([5c1a98a](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/5c1a98a0b6ef1536e3d64750758f5e8795ad47af)), closes [#13](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/issues/13)


### Bug Fixes

* decouple readiness from config key count ([7aa378a](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/7aa378aa341d89a6a03fef3ddb94b4aa40b3d50a)), closes [#12](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/issues/12)

## [0.10.0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/compare/kustomize-mutating-webhook-0.9.0...kustomize-mutating-webhook-v0.10.0) (2026-04-14)


### Features

* fix stability bugs and switch to native K8s config watching ([a1598c0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/a1598c0b94baa153126eab7479da834a842362f5))
* implement remaining review findings and best practices ([b0afa3b](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/b0afa3bb7016c4565d61e90c20239ab6736d2197))


### Bug Fixes

* address final review findings ([cfef9a2](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/cfef9a26d04de6a4108af06ce37abf6f9086d0eb))
* resolve shutdown races, JSON injection, and test quality issues ([e6b497d](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/e6b497d093d917d2c9d36b948d07b1fe60dfd104))


### Miscellaneous

* clean up review artifacts and fix remaining issues ([1bb18ab](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/1bb18abc9e1aa945462b3e036b47df55426a6b7e))

## [0.9.0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/compare/kustomize-mutating-webhook-v0.8.0...kustomize-mutating-webhook-v0.9.0) (2026-03-17)


### Features

* upgrade Go to 1.26.1 and update all dependencies ([f96c956](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/f96c956496e731dba0d88dd6f5a01f2c89ad5d2e))

## [0.8.0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/compare/kustomize-mutating-webhook-0.7.0...kustomize-mutating-webhook-v0.8.0) (2026-03-17)


### Features

* add better reloading logging ([9697bb4](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/9697bb48d9958b4439c35734b69a3bb3b8623366))
* add dynamic reloading ([bee3d28](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/bee3d281ee7e53180b7d01fc4461f10170e2517f))
* add kustomization reloading with annotation ([27333fe](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/27333fe179b6f3508e6251a2c9fb5ac90853bfd7))
* ensure configurations align ([fee1c33](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/fee1c337b526a3b6502e3177e12049766beb2019))
* refactor repository structure ([f8fc030](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/f8fc030ac04be285b39196ec06ba3155130412f7))


### Bug Fixes

* adding missing file ([4d6c14c](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/4d6c14cb6575817f5fde123d99ba30ce89f9675d))
* lock on mutation ([5d07a18](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/5d07a18fc8be89f4fdfda663bc9d54ab21c6a146))
* permissions ([38c1b8c](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/38c1b8c1ede187f1518009e4caabe544fcd346fa))
* recursively watch config subdirectories for hot-reload ([eaeae44](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/eaeae44504423e88c5abb5f2da0953eff2df9acc))


### Miscellaneous

* release main ([e1ee9d0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/e1ee9d0bd6b880aa0b88e239b58c3c2312c3c1c5))

## [0.6.0](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/compare/kustomize-mutating-webhook-0.5.0...kustomize-mutating-webhook-v0.6.0) (2025-10-17)


### Features

* ensure configurations align ([fee1c33](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/fee1c337b526a3b6502e3177e12049766beb2019))
* refactor repository structure ([f8fc030](https://github.com/xunholy/fluxcd-kustomize-mutating-webhook/commit/f8fc030ac04be285b39196ec06ba3155130412f7))
