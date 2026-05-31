# prepuller

Kubernetes controller and node agent for pre-pulling multi-architecture container images into node runtime caches.

## Image discovery operator

The initial operator watches Pods across the cluster. Whenever it observes a container, init container, or ephemeral container image reference, it creates a cluster-scoped `Image` custom resource in the `prepuller.theatreofdreams.dev/v1alpha1` API group.

Each `Image` resource is named with a stable hash of the original image reference and stores the original value in `spec.reference`. Platform-specific digests can be stored in `spec.platforms`.

```yaml
apiVersion: prepuller.theatreofdreams.dev/v1alpha1
kind: Image
metadata:
  name: img-...
spec:
  reference: nginx:1.27
  platforms:
    linux/amd64: nginx@sha256:...
    linux/arm64: nginx@sha256:...
```

## Development

```sh
go test ./...
```

## Deploy

```sh
kubectl apply -k config/default
```

The default deployment includes:

- a manager Deployment that discovers images from Pods and resolves platform digests into `Image` CRs
- a privileged node-agent DaemonSet that pulls desired digests into matching node runtimes
- conservative node GC that removes only images previously pulled by prepuller and no longer desired

Limit resolved platforms with `PREPULLER_PLATFORMS`:

```sh
PREPULLER_PLATFORMS=linux/amd64,linux/arm64 /manager
```

For local kind deploys:

```sh
make kind-up PLATFORMS=linux/amd64,linux/arm64
```

The kind cluster is created with `config/kind/cluster.yaml`, which configures node containerd to skip TLS verification for `registry.k8s.io`. This is required for the node agent path because image pulls happen through CRI on the node runtime, not through the manager's registry client.

If the kind cluster already exists, apply the same containerd setting in-place:

```sh
make kind-containerd-insecure
```

Limit node-agent activity with `PREPULLER_NODE_SELECTOR`:

```sh
PREPULLER_NODE_SELECTOR=kubernetes.io/os=linux /agent
```
