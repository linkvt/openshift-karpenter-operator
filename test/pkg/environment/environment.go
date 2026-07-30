package environment

import (
	"context"
	"fmt"

	"github.com/openshift/karpenter-operator/pkg/cloudprovider/common"
	"github.com/openshift/karpenter-operator/pkg/operator"
	testclient "github.com/openshift/karpenter-operator/test/pkg/client"

	configv1 "github.com/openshift/api/config/v1"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Environment holds shared state for all test suites. Constructed once in
// BeforeSuite and referenced via a package-level var in each suite.
type Environment struct {
	Client         client.Client
	Infrastructure common.InfrastructureInfo

	// ManagementCluster mirrors the MANAGEMENT_CLUSTER env var used to switch the
	// operator into HyperShift mode, so tests can skip suites that don't apply.
	ManagementCluster bool
}

// New creates an Environment by building a client and reading cluster metadata.
func New() (*Environment, error) {
	cl, err := testclient.NewClient()
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	managementCluster, err := operator.ParseManagementClusterEnv()
	if err != nil {
		return nil, err
	}

	if managementCluster {
		// TODO(maxcao13): We don't want to hardcode this, we want to read it from HCP object.
		// We are not running these tests on HCP yet, so it's okay for now.
		return &Environment{
			Client: cl,
			Infrastructure: common.InfrastructureInfo{
				PlatformType: configv1.PlatformType("AWS"),
			},
			ManagementCluster: true,
		}, nil
	}

	infra := &configv1.Infrastructure{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "cluster"}, infra); err != nil {
		return nil, fmt.Errorf("reading Infrastructure CR: %w", err)
	}

	env := &Environment{
		Client: cl,
		Infrastructure: common.InfrastructureInfo{
			InfraName: infra.Status.InfrastructureName,
		},
	}
	if infra.Status.PlatformStatus != nil {
		env.Infrastructure.PlatformType = infra.Status.PlatformStatus.Type
		if infra.Status.PlatformStatus.AWS != nil {
			env.Infrastructure.Region = infra.Status.PlatformStatus.AWS.Region
		}
	}
	if infra.Status.APIServerInternalURL != "" {
		env.Infrastructure.ClusterEndpoint = infra.Status.APIServerInternalURL
	}

	return env, nil
}

// IsAWSPlatform returns true if the cluster is on AWS.
func (e *Environment) IsAWSPlatform() bool {
	return e.Infrastructure.PlatformType == configv1.AWSPlatformType
}

// IsManagementCluster returns true if the tests are running against a HyperShift
// management cluster, mirroring the operator's own MANAGEMENT_CLUSTER env var.
func (e *Environment) IsManagementCluster() bool {
	return e.ManagementCluster
}
