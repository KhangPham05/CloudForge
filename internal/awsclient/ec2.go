package awsclient

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// VPCProvisioner is everything the VPC controller needs from AWS.
// (A VPC is part of the EC2 API family, hence this file's name.)
type VPCProvisioner interface {
	// FindVPCByName looks up a VPC by the Name tag we gave it, and
	// returns its AWS-generated ID if found. The controller uses this
	// before creating, so Reconcile is safe to run more than once for
	// the same object.
	FindVPCByName(ctx context.Context, name string) (vpcID string, found bool, err error)

	// CreateVPC creates a new VPC, tagged with name, and returns the
	// ID AWS assigned it. enableDNSSupport controls DNS resolution
	// within the VPC - AWS only exposes this as a separate call after
	// creation, not as part of CreateVpc itself, so this method makes
	// both calls so the caller doesn't have to sequence them.
	CreateVPC(ctx context.Context, name, cidrBlock string, enableDNSSupport bool) (vpcID string, err error)

	// DeleteVPC deletes a VPC by its AWS-generated ID.
	DeleteVPC(ctx context.Context, vpcID string) error
}

// ec2Client is the real, AWS-SDK-backed implementation of VPCProvisioner.
type ec2Client struct {
	client *ec2.Client
}

// NewEC2Client builds a VPCProvisioner backed by the real AWS SDK. cfg
// should come from NewAWSConfig, so it already has the LocalStack
// endpoint override applied when running locally.
func NewEC2Client(cfg aws.Config) VPCProvisioner {
	return &ec2Client{client: ec2.NewFromConfig(cfg)}
}

// FindVPCByName looks up a VPC by its Name tag.
func (c *ec2Client) FindVPCByName(ctx context.Context, name string) (string, bool, error) {
	out, err := c.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{name},
			},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("describing VPCs with name %q: %w", name, err)
	}

	if len(out.Vpcs) == 0 {
		return "", false, nil
	}

	return aws.ToString(out.Vpcs[0].VpcId), true, nil
}

// CreateVPC creates a new VPC, tags it with name, sets its DNS
// support attribute, and returns the ID AWS assigned it.
func (c *ec2Client) CreateVPC(ctx context.Context, name, cidrBlock string, enableDNSSupport bool) (string, error) {
	out, err := c.client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String(cidrBlock),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVpc,
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String(name)},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating VPC %q: %w", name, err)
	}

	vpcID := aws.ToString(out.Vpc.VpcId)

	_, err = c.client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId:            aws.String(vpcID),
		EnableDnsSupport: &types.AttributeBooleanValue{Value: aws.Bool(enableDNSSupport)},
	})
	if err != nil {
		return "", fmt.Errorf("setting DNS support on VPC %q: %w", vpcID, err)
	}

	return vpcID, nil
}

// DeleteVPC deletes a VPC by its AWS-generated ID.
func (c *ec2Client) DeleteVPC(ctx context.Context, vpcID string) error {
	_, err := c.client.DeleteVpc(ctx, &ec2.DeleteVpcInput{
		VpcId: aws.String(vpcID),
	})
	if err != nil {
		return fmt.Errorf("deleting VPC %q: %w", vpcID, err)
	}
	return nil
}
