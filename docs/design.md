# nyallocator Design Note

## Motivation

We are developing a Kubernetes operator to manage multiple node configurations in a declarative way.
This operator is intended to be used in conjunction with [CKE](https://github.com/cybozu-go/cke) for Node management and [Sabakan](https://github.com/cybozu-go/sabakan) for server management.

### Goal

- Provide a declarative way to manage node configurations.
- Support for multiple node types and configurations.
- Allow users to define node configurations by Kubernetes Custom Resource.

### Non-goals

- Creation of node objects.
- Management of node lifecycle.

## Design

### Overview

This operator allocates nodes from the “Spare nodes” and configures it as defined in the manifest.

"Spare nodes" are nodes that have a special taint that indicates they are not currently in use, and any pod except for DaemonSet pods have not been scheduled on them.
Taints that indicate spare nodes are expected to be set by the `--register-with-taint` option of `kubelet`. This option is used to set a taint on the node when it registers with the Kubernetes API server.
The taint key is `node.cybozu.io/spare`.

The operator will delete the taint when allocating the node from spare nodes, then the node will be available for scheduling pods.

### Custom Resource Definition (CRD)

We define a Custom Resource Definition (CRD) for the node configurations. This CRD is named `NodeTemplate` and will allow users to specify the desired state of node configurations.
Here is an example of a NodeTemplate. (Please see the [API](./api.md) for the full specification of the NodeTemplate.)

```yaml
kind: NodeTemplate
metadata:
  name: compute
spec:
  dry-run: false
  priority: 1
  selector:
    matchExpressions:
 - {key: cke.cybozu.com/role, operator: In, values: [compute]}
 - {key: cke.cybozu.com/master, operator: NotIn, values: [true]}
  nodes: 10
  template:
    metadata:
      annotations:
        for: bar
      labels:
        baz: qux
    spec:
      taints:
      - key: foo
        value: bar
        effect: NoSchedule
```

This NodeTemplate defines a node configuration for compute nodes. It specifies that the operator should allocate 10 nodes that match the selector, and apply the specified annotations, labels, and taints to the nodes.

### Operator Logic

The operator consists of two reconcilers: `NodeTemplateReconciler` and `NodeReconciler`.
The `NodeTemplate` reconciler is responsible for managing the NodeTemplate resources, while the `Node` reconciler is responsible for managing spare nodes.

#### Node Reconciler

The `Node` reconciler will watch for changes to `Node`. If the node has a spare taint, it will add a label that indicates the node is spare for the purpose of easy identification of spare nodes when executing `kubectl get nodes`.
The spare label is `node-role.kubernetes.io/spare: "true"`.

#### NodeTemplate Reconciler

The NodeTemplate reconciler watches for changes to NodeTemplate resources and Node resources.
The reconciler will be triggered by the following events:

- When a NodeTemplate is updated, reconcile the NodeTemplate.
- When a Node's labels, annotations, and taints are updated, reconcile the NodeTemplate that is described in the reference label.

The logic of reconciliation is as follows:

- It checks the node under the NodeTemplate and if there are diffs between the desired state and the actual state of the nodes, it will update the nodes accordingly.
- If the number of nodes that is under the NodeTemplate is less than the desired number, it will try to allocate new nodes from the spare nodes. The steps of allocation are as follows:
  - Delete the taint and label that indicates the node is spare.
  - Add the label that indicates the node is under the NodeTemplate.
  - Configure the node's annotations, labels, and taints according to the NodeTemplate's specification.
