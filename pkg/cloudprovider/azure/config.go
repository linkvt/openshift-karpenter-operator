package azure

import (
	"github.com/openshift/karpenter-operator/pkg/assets"
	"github.com/openshift/karpenter-operator/pkg/cloudprovider/common"

	configv1 "github.com/openshift/api/config/v1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (p *Provider) AddToScheme(_ *runtime.Scheme) error {
	return nil
}

func (p *Provider) KarpenterImage() string {
	return p.karpenterImage
}

func (p *Provider) OperandConfig() common.OperandCloudConfig {
	return common.OperandCloudConfig{}
}

func (p *Provider) CRDs() []*apiextensionsv1.CustomResourceDefinition {
	return assets.AzureCRDs
}

func (p *Provider) RBAC() common.RBACAssets {
	return common.RBACAssets{}
}

func (p *Provider) RelatedObjects() []configv1.ObjectReference {
	return []configv1.ObjectReference{
		{Group: "karpenter.azure.com", Resource: "aksnodeclasses"},
	}
}
