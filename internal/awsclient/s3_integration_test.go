//go:build integration

package awsclient

import (
	"context"
	"testing"
)

// TestS3Client_CreateHeadDelete exercises the real S3Provisioner
// against a live LocalStack container. Requires AWS_ENDPOINT_URL (and
// dummy AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_REGION) to be set,
// and LocalStack running locally - this does not run as part of a
// normal `go test ./...`.
func TestS3Client_CreateHeadDelete(t *testing.T) {
	ctx := context.Background()

	cfg, err := NewAWSConfig(ctx)
	if err != nil {
		t.Fatalf("building AWS config: %v", err)
	}

	client := NewS3Client(cfg)
	const bucketName = "cloudforge-phase5-test-bucket"

	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		t.Fatalf("BucketExists before create: %v", err)
	}
	if exists {
		t.Fatalf("expected bucket %q not to exist yet", bucketName)
	}

	if err := client.CreateBucket(ctx, bucketName); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	exists, err = client.BucketExists(ctx, bucketName)
	if err != nil {
		t.Fatalf("BucketExists after create: %v", err)
	}
	if !exists {
		t.Fatalf("expected bucket %q to exist after create", bucketName)
	}

	if err := client.DeleteBucket(ctx, bucketName); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}

	exists, err = client.BucketExists(ctx, bucketName)
	if err != nil {
		t.Fatalf("BucketExists after delete: %v", err)
	}
	if exists {
		t.Fatalf("expected bucket %q not to exist after delete", bucketName)
	}
}
