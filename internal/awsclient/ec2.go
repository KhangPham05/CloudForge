package awsclient

import "context"

// VPCProvisioner is everything the VPC controller needs from AWS.
// (A VPC is part of the EC2 API family, hence this file's name.)
type VPCProvisioner interface {
	// FindVPCByName looks up a VPC by the Name tag we gave it, and
	// returns its AWS-generated ID if found. The controller uses this
	// before creating, so Reconcile is safe to run more than once for
	// the same object.
	FindVPCByName(ctx context.Context, name string) (vpcID string, found bool, err error)

	// CreateVPC creates a new VPC and returns the ID AWS assigned it.
	CreateVPC(ctx context.Context, name, cidrBlock string) (vpcID string, err error)

	// DeleteVPC deletes a VPC by its AWS-generated ID.
	DeleteVPC(ctx context.Context, vpcID string) error
}
