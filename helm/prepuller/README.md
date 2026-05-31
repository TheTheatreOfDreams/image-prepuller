# prepuller Helm chart

This chart installs the prepuller `Image` CRD, controller manager, node agent, and RBAC.

## Placement model

Run the controller as a small singleton workload on regular cluster nodes. It only watches Pods and writes `Image` custom resources, so it does not need privileged access or host mounts.

Run the agent only on nodes whose runtime cache should be warmed. The agent is privileged, mounts the node runtime socket, and uses a hostPath state directory, so schedule it deliberately with `agent.nodeSelector`, `agent.tolerations`, and `agent.affinity`.

The `agent` is a Deployment. Use `agent.replicas` to choose how many agents should run in the selected node group. Agent pods always include hard pod anti-affinity, so multiple replicas must land on separate nodes.

There are two selector concepts for agents:

- `agent.nodeSelector` controls where Kubernetes schedules the agent pods.
- `agent.managedNodeSelector` is passed to the agent process and controls whether that node actually prepulls images.

In most clusters, set `agent.nodeSelector` broadly enough to cover the runtime family and use `agent.managedNodeSelector` for the prepuller opt-in label. For example:

```yaml
agent:
  nodeSelector:
    kubernetes.io/os: linux
  managedNodeSelector: prepuller.theatreofdreams.dev/enabled=true
```

For exactly two agents in a specific node group, each on a separate node:

```yaml
agent:
  replicas: 2
  nodeSelector:
    nodepool: image-cache
  managedNodeSelector: prepuller.theatreofdreams.dev/enabled=true
```

## Install

```sh
helm upgrade --install prepuller ./helm/prepuller --namespace prepuller-system --create-namespace
```

## Seed nodes

Seed nodes pull every platform digest from every `Image` CR. Label those nodes and keep `agent.seedNodeSelector` aligned:

```sh
kubectl label node <node> prepuller.theatreofdreams.dev/seed=true
```
