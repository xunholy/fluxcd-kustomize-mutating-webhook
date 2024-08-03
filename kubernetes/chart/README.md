# kustomize-mutating-webhook

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

A Helm chart for FluxCD Kustomize Mutating Webhook

## Introduction

This Helm chart deploys a mutating webhook for FluxCD Kustomization resources. It extends the functionality of postBuild substitutions beyond the scope of a single namespace, allowing the use of global configuration variables stored in a central namespace.

## Prerequisites

- Kubernetes 