Maintenance
===========

This page describes how to update nyallocator regularly.

## Prerequisites

CLI tools (gh, yq, kind, kustomize, kubectl, setup-envtest, controller-gen) are managed by [aqua](https://aquaproj.github.io/) in `aqua.yaml`.
Their checksums are recorded in `aqua-checksums.json` and verified on installation.
controller-gen is an exception: it is installed with `go install` by aqua, so it is verified with the Go checksum database instead of `aqua-checksums.json`.
The Makefiles execute the tools via `aqua exec`, so install aqua in advance and put the `aqua` command in a directory included in `PATH`.
See [Install aqua](https://aquaproj.github.io/docs/install) for details.
Then run `make setup` to install the tools.

## Procedure

1. Update `go.mod`. In addition to go modules, update the Go version if needed.
2. Update GitHub Actions and Ubuntu versions in `.github/workflows/*.yaml` if needed.
3. Update the base image of the build stage in `Dockerfile`.
   Use the image of the same Go version as the `go` directive in `go.mod`.
4. Update the tools managed by aqua.
   ```console
   $ make maintenance
   ```
   This updates the following:
   - gh, yq, kind, kustomize and the aqua standard registry in `aqua.yaml`, and `aqua-checksums.json`
   - aqua version in `.github/workflows/*.yaml`

   Only versions released at least 14 days ago are adopted.
   The cooldown period can be changed with `COOLDOWN_DAYS` (e.g. `make maintenance COOLDOWN_DAYS=7`).
5. Update `kubernetes-sigs/controller-runtime/setup-envtest` in `aqua.yaml` to the same version as controller-runtime in `go.mod`.
   Also update `kubernetes-sigs/controller-tools/controller-gen` in `aqua.yaml` to the version that supports the Kubernetes version of `k8s.io/api` in `go.mod`.
   The Kubernetes version for envtest is derived from `k8s.io/api` in `go.mod`.
   ```console
   $ # Use this command to list the available k8s versions for envtest
   $ aqua exec -- setup-envtest list
   ```
6. Update `E2ETEST_K8S_VERSION` and `E2ETEST_KINDEST_NODE_IMAGE_HASH` in `e2e/Makefile.versions`.
   Specify the version of `kindest/node` supported by the kind written in `aqua.yaml`.
   Also update `kubernetes/kubectl` in `aqua.yaml` to the same version as `E2ETEST_K8S_VERSION`.
7. If you changed `aqua.yaml` manually, update `aqua-checksums.json`.
   Since `require_checksum` is enabled, packages without checksums fail to install.
   ```console
   $ aqua update-checksum -prune
   ```
   See also [Notes on editing aqua.yaml manually](#notes-on-editing-aquayaml-manually).
8. Run `make check-generate` and commit the regenerated files if any.
9. Follow [release.md](/docs/release.md) to update software version.

## Notes on editing aqua.yaml manually

- Update only `kubernetes/kubectl`, `kubernetes-sigs/controller-runtime/setup-envtest` and `kubernetes-sigs/controller-tools/controller-gen` manually.
  The other packages and the standard registry are updated by `make maintenance` with the cooldown period.
- Write the version as the tag name, including its prefix if any (e.g. `kubernetes-sigs/kustomize@kustomize/v5.8.3`).
- You can select a version interactively with `aqua update -s <package>` (e.g. `aqua update -s kubernetes/kubectl`).
  Do not run `aqua update` without arguments, because it updates all packages to the latest versions ignoring the cooldown period.
- After editing, update `aqua-checksums.json` with `aqua update-checksum -prune`.
