// Package fake provides in-memory implementations of the awsclient
// provisioner interfaces, for fast, hermetic tests that don't need a
// real LocalStack container running.
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/KhangPham05/cloudforge/internal/awsclient"
)

// S3Provisioner is an in-memory fake implementing awsclient.S3Provisioner.
type S3Provisioner struct {
	mu      sync.Mutex
	buckets map[string]bool
}

// compile-time check that S3Provisioner satisfies the real interface.
var _ awsclient.S3Provisioner = (*S3Provisioner)(nil)

// NewS3Provisioner builds an empty fake S3Provisioner.
func NewS3Provisioner() *S3Provisioner {
	return &S3Provisioner{buckets: make(map[string]bool)}
}

func (f *S3Provisioner) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buckets[bucketName], nil
}

func (f *S3Provisioner) CreateBucket(ctx context.Context, bucketName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.buckets[bucketName] {
		return fmt.Errorf("bucket %q already exists", bucketName)
	}
	f.buckets[bucketName] = true
	return nil
}

func (f *S3Provisioner) DeleteBucket(ctx context.Context, bucketName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.buckets[bucketName] {
		return fmt.Errorf("bucket %q does not exist", bucketName)
	}
	delete(f.buckets, bucketName)
	return nil
}
