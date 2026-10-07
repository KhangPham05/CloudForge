package awsclient

import "context"

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
