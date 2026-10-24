//go:build integration

package awsclient

import (
	"context"
	"testing"
)

// TestEC2Client_FindCreateDelete exercises the real VPCProvisioner
// against a live LocalStack container. Requires AWS_ENDPOINT_URL (and
// dummy AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_REGION) to be set,
// and LocalStack running locally - this does not run as part of a
// normal `go test ./...`.
func TestEC2Client_FindCreateDelete(t *testing.T) {
	ctx := context.Background()

	cfg, err := NewAWSConfig(ctx)
	if err != nil {
		t.Fatalf("building AWS config: %v", err)
	}

	client := NewEC2Client(cfg)
	const vpcName = "cloudforge-phase13-test-vpc"
	const cidrBlock = "10.42.0.0/16"

	_, found, err := client.FindVPCByName(ctx, vpcName)
	if err != nil {
		t.Fatalf("FindVPCByName before create: %v", err)
	}
	if found {
		t.Fatalf("expected VPC %q not to exist yet", vpcName)
	}

	vpcID, err := client.CreateVPC(ctx, vpcName, cidrBlock, true)
	if err != nil {
		t.Fatalf("CreateVPC: %v", err)
	}
	if vpcID == "" {
		t.Fatal("expected CreateVPC to return a non-empty VPC ID")
	}

	foundID, found, err := client.FindVPCByName(ctx, vpcName)
	if err != nil {
		t.Fatalf("FindVPCByName after create: %v", err)
	}
	if !found {
		t.Fatalf("expected VPC %q to exist after create", vpcName)
	}
	if foundID != vpcID {
		t.Fatalf("expected found VPC ID %q to match created VPC ID %q", foundID, vpcID)
	}

	if err := client.DeleteVPC(ctx, vpcID); err != nil {
		t.Fatalf("DeleteVPC: %v", err)
	}

	_, found, err = client.FindVPCByName(ctx, vpcName)
	if err != nil {
		t.Fatalf("FindVPCByName after delete: %v", err)
	}
	if found {
		t.Fatalf("expected VPC %q not to exist after delete", vpcName)
	}
}
