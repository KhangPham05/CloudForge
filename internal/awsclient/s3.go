package awsclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Provisioner is everything the S3Bucket controller needs from AWS S3.
// The controller depends on this interface, never on a concrete SDK
// client directly - that's what lets us swap in a fake for tests.
type S3Provisioner interface {
	// BucketExists reports whether a bucket with this name already
	// exists. The controller uses this before creating, so Reconcile
	// is safe to run more than once for the same object.
	BucketExists(ctx context.Context, bucketName string) (bool, error)

	// CreateBucket creates a new S3 bucket.
	CreateBucket(ctx context.Context, bucketName string) error

	// DeleteBucket deletes an S3 bucket. The bucket must already be
	// empty - the controller's finalizer is responsible for emptying
	// it first.
	DeleteBucket(ctx context.Context, bucketName string) error
}

// s3Client is the real, AWS-SDK-backed implementation of S3Provisioner.
type s3Client struct {
	client *s3.Client
}

// NewS3Client builds an S3Provisioner backed by the real AWS SDK. cfg
// should come from NewAWSConfig, so it already has the LocalStack
// endpoint override applied when running locally.
func NewS3Client(cfg aws.Config) S3Provisioner {
	return &s3Client{client: s3.NewFromConfig(cfg, func(o *s3.Options) {
		// S3 normally addresses buckets as "bucketname.s3.amazonaws.com"
		// (virtual-hosted style), which doesn't resolve against
		// "localhost". Path-style ("localhost:4566/bucketname") works
		// against any S3-compatible endpoint, including LocalStack.
		o.UsePathStyle = true
	})}
}

// BucketExists reports whether a bucket with this name already exists.
func (c *s3Client) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		return true, nil
	}

	if _, ok := errors.AsType[*types.NotFound](err); ok {
		return false, nil
	}

	return false, fmt.Errorf("checking if bucket %q exists: %w", bucketName, err)
}

// CreateBucket creates a new S3 bucket.
func (c *s3Client) CreateBucket(ctx context.Context, bucketName string) error {
	_, err := c.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("creating bucket %q: %w", bucketName, err)
	}
	return nil
}

// DeleteBucket deletes an S3 bucket. The caller is responsible for
// making sure it's already empty - S3 refuses to delete a non-empty
// bucket.
func (c *s3Client) DeleteBucket(ctx context.Context, bucketName string) error {
	_, err := c.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("deleting bucket %q: %w", bucketName, err)
	}
	return nil
}
