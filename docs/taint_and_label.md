# Label and Taints used by nyallocator

This document describes the labels and taints used by the nyallocator.

## Labels

### node-role.kubernetes.io/spare

This label will be added automatically to nodes that have a spare taint.
This label is used to identify spare nodes when executing `kubectl get nodes`.

### nyallocator.cybozu.io/node-template

This label is used to indicate that a node is managed by the NodeTemplate. Value of this label is the name of the NodeTemplate that manages the node.

There are ValidatingAdmissionPolicy not to allow edit this label except for the operator.
The example is placed in the [`config/vap`](../config/vap) directory.

### nyallocator.cybozu.io/out-of-management

This label is used to indicate that a node is out of management by the NodeTemplate.

Out of management node means that the node that has a `nyallocator.cybozu.io/node-template` label but does not match the NodeTemplate's selector.
In such situation, nyallocator deletes the `nyallocator.cybozu.io/node-template` label from the node and adds this label.

This condition can happen when the NodeTemplate's selector is updated and the node no longer matches the new selector.

If this label is added to a node, user should check the node and update the node or delete the node if it is no longer needed.

### node-role.kubernetes.io/out-of-management

Same as `nyallocator.cybozu.io/out-of-management`, but this label is used to identify out of management nodes when executing `kubectl get nodes`.

## Taints

### node.cybozu.io/spare

This label is used to identify spare nodes in the cluster.
This taint is expected to be set by the `--register-with-taint` option of `kubelet`.
