// Package awsclient wraps all AWS SDK access for CloudForge's controllers.
// It is the only package in this project that imports aws-sdk-go-v2 directly.
package awsclient

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// NewAWSConfig builds an aws.Config for the SDK clients to use.
//
// If AWS_ENDPOINT_URL is set (as it is during local dev, pointing at
// LocalStack), every AWS API call is redirected there instead of to
// real AWS. If it's unset, the SDK falls back to its normal default
// behavior, which is what we want in production.
func NewAWSConfig(ctx context.Context) (aws.Config, error) {
	var optFns []func(*config.LoadOptions) error

	if endpoint := os.Getenv("AWS_ENDPOINT_URL"); endpoint != "" {
		optFns = append(optFns, config.WithBaseEndpoint(endpoint))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}

	return cfg, nil
}
