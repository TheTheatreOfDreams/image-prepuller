# image-prepuller

Kubernetes controller and node agent for pre-pulling multi-architecture container images into node runtime caches.

## Image discovery operator

The initial operator watches Pods across the cluster. Whenever it observes a container, init container, or ephemeral container image reference, it creates a cluster-scoped `Image` custom resource in the `image-prepuller.theatreofdreams.io/v1alpha1` API group.

Each `Image` resource is named with a stable hash of the original image reference and stores the original value in `spec.reference`. Platform-specific digests can be stored in `spec.platforms`.

```yaml
apiVersion: image-prepuller.theatreofdreams.io/v1alpha1
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
