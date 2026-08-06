package aws

// Environment variables used when deploying AWS Karpenter.
const (
	// KarpenterImageEnvName is the environment variable name pointing to the AWS Karpenter image
	// to be deployed by the operator.
	KarpenterImageEnvName = "KARPENTER_IMAGE_AWS"

	// AWSSharedAuthFileEnvName is the environment variable name of the path that points
	// to the AWS shared credentials file mounted in the operand pod.
	AWSSharedAuthFileEnvName = "AWS_SHARED_CREDENTIALS_FILE"
)
