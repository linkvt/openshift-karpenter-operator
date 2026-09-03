package azure

// Environment variables used when deploying Azure Karpenter.
// HyperShift injects these into the operator pod; the provider forwards
// operand-facing vars to the karpenter-provider-azure deployment.
const (
	// KarpenterImageEnvName is the environment variable name pointing to the Azure Karpenter image
	// to be deployed by the operator.
	KarpenterImageEnvName = "KARPENTER_IMAGE_AZURE"

	// AzureClientIDEnvName is the Azure workload identity client ID.
	AzureClientIDEnvName = "AZURE_CLIENT_ID"

	// AzureTenantIDEnvName is the Azure tenant ID.
	AzureTenantIDEnvName = "AZURE_TENANT_ID"

	// AzureSubscriptionIDEnvName is the Azure subscription ID.
	AzureSubscriptionIDEnvName = "AZURE_SUBSCRIPTION_ID"

	// AzureFederatedTokenFileEnvName is the path to the federated workload identity token
	// minted for kube-system/karpenter. In HCP this is the token-minter output at
	// /var/run/secrets/openshift/serviceaccount/token.
	AzureFederatedTokenFileEnvName = "AZURE_FEDERATED_TOKEN_FILE"

	// AzureVNetSubnetIDEnvName is the ARM resource ID of the subnet new nodes attach to.
	// Format: /subscriptions/{id}/resourceGroups/{rg}/providers/Microsoft.Network/virtualNetworks/{vnet}/subnets/{subnet}
	AzureVNetSubnetIDEnvName = "VNET_SUBNET_ID"

	// AzureNodeResourceGroupEnvName is the Azure resource group where Karpenter creates VMs.
	// AKS calls this the node/MC resource group. On HCP this is
	// HostedCluster.Spec.Platform.Azure.ResourceGroupName.
	AzureNodeResourceGroupEnvName = "AZURE_NODE_RESOURCE_GROUP"

	// AzureLocationEnvName is the Azure location env var the operand's Azure SDK reads.
	AzureLocationEnvName = "LOCATION"

	// AzureProvisionModeEnvName is the karpenter-provider-azure provisioning mode.
	AzureProvisionModeEnvName = "PROVISION_MODE"
)
