package operator

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestIsPlatformSupported(t *testing.T) {
	testCases := map[string]struct {
		managementCluster bool
		platform          configv1.PlatformType
		want              bool
	}{
		"When running standalone on AWS, it should be supported": {
			platform: configv1.AWSPlatformType,
			want:     true,
		},
		"When running standalone on Azure, it should not be supported": {
			platform: configv1.AzurePlatformType,
		},
		"When running standalone on platform None, it should not be supported": {
			platform: configv1.NonePlatformType,
		},
		"When running standalone with an empty platform type, it should not be supported": {
			platform: "",
		},
		"When running on a management cluster, it should be supported regardless of platform": {
			managementCluster: true,
			platform:          configv1.BareMetalPlatformType,
			want:              true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			if got := isPlatformSupported(tc.managementCluster, tc.platform); got != tc.want {
				t.Errorf("isPlatformSupported(%t, %q) = %t, want %t", tc.managementCluster, tc.platform, got, tc.want)
			}
		})
	}
}
