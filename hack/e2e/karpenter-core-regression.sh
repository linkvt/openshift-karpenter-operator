#!/usr/bin/env bash
set -euo pipefail

export CLUSTER="$(<"${SHARED_DIR}/cluster-name")"
export HYPERSHIFT_NAMESPACE="$(<"${SHARED_DIR}/management_cluster_namespace")"

# Clone karpenter core fork and vendor it
dir=$(mktemp -d)
git clone https://github.com/openshift/kubernetes-sigs-karpenter.git --depth=1 "$dir"
go -C "$dir" mod vendor

# Annotate HCP on management cluster to enable e2e override
export KUBECONFIG=${SHARED_DIR}/management_cluster_kubeconfig
oc annotate -n "${HYPERSHIFT_NAMESPACE}-${CLUSTER}" "hcp/${CLUSTER}" hypershift.openshift.io/karpenter-core-e2e-override=true

# Switch to guest cluster
export KUBECONFIG=${SHARED_DIR}/nested_kubeconfig
oc delete validatingadmissionpolicybindings.admissionregistration.k8s.io karpenter-binding.ec2nodeclass.hypershift.io

KARPENTER_CORE_DIR=$dir TEST_SUITE=regression ./hack/e2e/upstream-e2e.sh
