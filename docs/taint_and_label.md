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

## Taints

### node.cybozu.io/spare

This label is used to identify spare nodes in the cluster.
This taint is expected to be set by the `--register-with-taint` option of `kubelet`.
