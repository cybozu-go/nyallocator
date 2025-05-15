## API Specification

## NoteTemplate

 field   |        type        | required |                       description
-------- | ------------------ | -------- | --------------------------------------------------------
metadata | metav1.Objectmeta  | yes      | Standard object metadata.
spec     | NoteTemplateSpec   | yes      | Specification of the desired state of the node template.
status   | NoteTemplateStatus | no       | Current status of the node template.

### NoteTemplateSpec

 field   |         type         | required |                                           description
-------- | -------------------- | -------- | -----------------------------------------------------------------------------------------------
dryRun   | boolean              | no       | If true, the operator will not apply the changes to the nodes. Default is false.
priority | integer              | no       | Priority of the node template. Higher priority templates will be processed first. Default is 1.
selector | metav1.LabelSelector | yes      | Label selector to match nodes. If not specified, all nodes will be matched.
nodes    | integer              | yes      | Number of nodes to allocate. It must be greater than or equal to 1.
template | NodeConfiguration    | yes      | Specification of the node template to apply to the nodes.

### NodeConfiguration

 field   |           type            | required |                        description
-------- | ------------------------- | -------- | ---------------------------------------------------------
metadata | NodeConfigurationMetadata | no       | Metadata to apply to the nodes, labels and annotations.
spec     | NodeConfigurationSpec     | no       | Specification of the node template to apply to the nodes.

### NodeConfigurationMetadata

   field    |       type        | required |            description
----------- | ----------------- | -------- | ----------------------------------
annotations | map[string]string | no       | Annotations to apply to the nodes.
labels      | map[string]string | no       | Labels to apply to the nodes.

### NodeConfigurationSpec

field  |      type      | required |              description
------ | -------------- | -------- | -------------------------------------
taints | []corev1.Taint | no       | List of taints to apply to the nodes.

### NoteTemplateStatus

   field      |      type      | required |                                                 description
------------- | -------------- | -------- | -----------------------------------------------------------------------------------------------------------
taints        | []corev1.Taint | no       | List of taints to apply to the nodes.
currentNodes  | integer        | no       | Number of allocated nodes that are currently available.
sufficient    | boolean        | no       | Indicates whether the current number of available nodes is sufficient to meet the expected number of nodes.
reconcileInfo | ReconcileInfo  | no       | Information about the reconciliation process.

## ReconcileInfo

  field    |  type   | required |                     description
---------- | ------- | -------- | ---------------------------------------------------
generation | integer | no       | The generation of the resource that was reconciled.
