# karpenter-operator

`karpenter-operator` deploys and manages [Red Hat build of Karpenter](https://www.redhat.com/en/blog/introducing-red-hat-build-karpenter) on OpenShift.
It installs the Karpenter CRDs and deploys the Karpenter operand.
On standalone clusters, it also reports operator health through a `ClusterOperator` object.

The operator supports two modes:

- **Standalone:** a cluster-scoped `Karpenter` custom resource (`autoscaling.openshift.io/v1alpha1`) named `default` drives operand deployment.
- **Hosted Control Plane:** the operator runs in the management cluster, installs Karpenter CRDs into the hosted cluster through a separate client, and deploys an operand for each `HostedControlPlane` configured to use Karpenter.

The operator currently supports AWS through the cloud-provider abstraction in `pkg/cloudprovider`.

## Getting started

For a minimal standalone configuration, see [examples/karpenter.yaml](./examples/karpenter.yaml).

## Related projects

- [openshift/aws-karpenter-provider-aws](https://github.com/openshift/aws-karpenter-provider-aws) — Karpenter operand
- [openshift/kubernetes-sigs-karpenter](https://github.com/openshift/kubernetes-sigs-karpenter) — Karpenter core fork

## Documentation

- [Getting Started with Red Hat Build of Karpenter (AutoNode) on ROSA](https://cloud.redhat.com/experts/rosa/karpenter/)
- [Introducing Red Hat build of Karpenter](https://www.redhat.com/en/blog/introducing-red-hat-build-karpenter)
- [Contributing](./CONTRIBUTING.md)
- [AI agent guidance](./AGENTS.md)
- [OpenShift CI configuration](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/karpenter-operator)
