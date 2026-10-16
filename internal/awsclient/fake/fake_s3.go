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
// Each bucket tracks its own set of object keys, so EmptyBucket has
// real behavior to verify in tests, not just a no-op.
type S3Provisioner struct {
	mu      sync.Mutex
	buckets map[string]map[string]bool // bucket name -> set of object keys
}

// compile-time check that S3Provisioner satisfies the real interface.
var _ awsclient.S3Provisioner = (*S3Provisioner)(nil)

// NewS3Provisioner builds an empty fake S3Provisioner.
func NewS3Provisioner() *S3Provisioner {
	return &S3Provisioner{buckets: make(map[string]map[string]bool)}
}

// PutObject is a test-only helper (not part of S3Provisioner) that
// lets a test put a fake bucket into a non-empty state, so it can
// verify EmptyBucket actually clears it out.
func (f *S3Provisioner) PutObject(bucketName, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.buckets[bucketName] == nil {
		f.buckets[bucketName] = make(map[string]bool)
	}
	f.buckets[bucketName][key] = true
}

func (f *S3Provisioner) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.buckets[bucketName]
	return ok, nil
}

func (f *S3Provisioner) CreateBucket(ctx context.Context, bucketName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.buckets[bucketName]; ok {
		return fmt.Errorf("bucket %q already exists", bucketName)
	}
	f.buckets[bucketName] = make(map[string]bool)
	return nil
}

func (f *S3Provisioner) EmptyBucket(ctx context.Context, bucketName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	objects, ok := f.buckets[bucketName]
	if !ok {
		return fmt.Errorf("bucket %q does not exist", bucketName)
	}
	for key := range objects {
		delete(objects, key)
	}
	return nil
}

func (f *S3Provisioner) DeleteBucket(ctx context.Context, bucketName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.buckets[bucketName]; !ok {
		return fmt.Errorf("bucket %q does not exist", bucketName)
	}
	delete(f.buckets, bucketName)
	return nil
}
