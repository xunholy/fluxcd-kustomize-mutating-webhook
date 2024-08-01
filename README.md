# FluxCD Kustomize Mutating Webhook

This project introduces a mutating webhook for FluxCD Kustomization, designed to extend the functionality of postBuild substitutions beyond the scope of a single namespace. It allows for the use of global configuration variables stored in a central namespace, thus avoiding the need for recursive Kustomize patches or duplicated secrets across multiple namespaces.

## Why This Webhook?

In standard FluxCD setups, postBuild substitutions in Kustomizations can only reference secrets within the same namespace. This limitation can lead to redundancy and complexity, particularly in environments requiring consistent configuration across multiple namespaces. This mutating webhook addresses this challenge by enabling global configuration management, significantly simplifying the process.

## How It Works

The webhook listens for Kustomization resources creation or update events. On intercepting such an event, it dynamically injects substitution variables into the Kustomization resource. These variables are fetched from a centralized ConfigMap, allowing for consistent and centralized management of configurations used across various namespaces.

## Prerequisites

The Kustomize Mutating Webhook is pre-configured to mount the configmap called `cluster-config` however, this can be set to any name. Ensure this exists in the cluster otherwise there will be no values to patch into your FluxCD Kustomization resources. Also see [Changing ConfigMap Reference](#changing-configmap-reference)

Additionally, the following are required:

* A Kubernetes Cluster with FluxCD installed
* Access (RBAC) to install a Kubernetes Mutating Webhook

## Installation

Using Kubernetes Manifests

1. Clone the Repository

```bash
git clone https://github.com/xunholy/fluxcd-kustomize-mutating-webhook.git
cd fluxcd-kustomize-mutating-webhook
```

2. Apply the Kubernetes Manifests

This will set up the mutating webhook in your cluster.

```bash
kubectl apply -k kubernetes/static
```

3. Verify Installation

Check if the webhook service and deployment are running correctly.

```bash
kubectl get pods --selector=app=kustomize-mutating-webhook -n flux-system
```

## Usage

### Changing the Log Level

The log level of the webhook can be adjusted to control the verbosity of the logs. This is useful for debugging or reducing the amount of log output in a production environment.

1. Update the Log Level Environment Variable

The log level is controlled by the LOG_LEVEL environment variable within the webhook's deployment. To change it, edit the deployment and set the LOG_LEVEL environment variable to one of the following: `debug`, `info`, `warn`, `error`, `fatal`, or `panic`.

Example:

```yaml
env:
- name: LOG_LEVEL
  value: "debug"
```

2. Apply the Changes

After editing the deployment, apply the changes to your cluster:

```bash
kubectl apply -k kubernetes/static
```

3. Verify the Changes

Check the logs of the webhook to ensure that the log level has changed:

```bash
kubectl logs --selector=app=kustomize-mutating-webhook -n flux-system
```

### Changing ConfigMap Reference

The webhook is designed to fetch substitution variables from a specified ConfigMap. To change the ConfigMap it references:

1. Update the ConfigMap Kubernetes Deployment

Example:

```yaml
volumes:
- name: cluster-config
  configMap:
    name: cluster-config
```

**Note:** *Only changing the `volumes.[0].name.configMap.name` is required.*

2. Apply the changes to the Deployment.

```bash
kubectl apply -k kubernetes/static
```

3. Verify the Changes

You can verify the correct values are being collected by either using the `debug` log level which outputs the values on start-up, alternatively you may also verify by inspecting a Kustomization resource that has been mutated.

## Testing and Benchmarking

This project includes unit tests and benchmarks to ensure reliability and performance. Here's how to run them and interpret the results:

### Running Tests

To run the unit tests, use the following command in the project root:

```bash
go test -v ./...
```

This will run all tests and provide verbose output. A successful test run will show "PASS" for each test case.

### Running Benchmarks

To run the benchmarks, use:

```bash
go test -bench=. -benchmem
```

This command runs all benchmarks and includes memory allocation statistics.

### Interpreting Results

#### Test Results

After running the tests, you should see output similar to:

```log
=== RUN   TestMutatingWebhook
=== RUN   TestMutatingWebhook/Add_postBuild_and_substitute
[... more test output ...]
PASS
ok      github.com/xunholy/fluxcd-mutating-webhook      0.015s
```

- "PASS" indicates all tests have passed successfully.
- The time at the end (0.015s in this example) shows how long the tests took to run.

#### Benchmark Results

Benchmark results will look something like this:

```log
4:47PM INF Request details Kind=Kustomization Name= Namespace= Resource= UID=
   25410             41239 ns/op
PASS
ok      github.com/xunholy/fluxcd-mutating-webhook      1.535s
```

Here's how to interpret these results:

- The first line shows a log output from the benchmark run.
- "25410" is the number of iterations the benchmark ran.
- "41239 ns/op" means each operation took an average of 41,239 nanoseconds (about 0.04 milliseconds).
- "PASS" indicates the benchmark completed successfully.
- "1.535s" is the total time taken for all benchmark runs.

### Importance of Testing and Benchmarking

Regular testing and benchmarking are crucial for several reasons:

1. **Reliability**: Tests ensure that the webhook behaves correctly under various scenarios.
2. **Performance Monitoring**: Benchmarks help track the webhook's performance over time, allowing us to detect and address any performance regressions.
3. **Optimization**: Benchmark results can guide optimization efforts by identifying slow operations.
4. **Confidence in Changes**: Running tests and benchmarks before and after changes helps ensure that modifications don't introduce bugs or performance issues.

We encourage contributors to run tests and benchmarks locally before submitting pull requests, and to include new tests for any added functionality.


## License

Distributed under the Apache 2.0 License. See [LICENSE](./LICENSE) for more information.
