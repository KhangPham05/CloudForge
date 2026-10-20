package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/KhangPham05/cloudforge/internal/awsclient"
)

// VPCProvisioner is an in-memory fake implementing awsclient.VPCProvisioner.
// Unlike the S3 fake, this one has to generate its own fake IDs - a
// real VPC's identity is an AWS-generated ID, not the name we gave
// it, so a faithful fake has to reproduce that same asymmetry.
type VPCProvisioner struct {
	mu     sync.Mutex
	nextID int
	vpcs   map[string]string // name -> fake VPC ID
}

// compile-time check that VPCProvisioner satisfies the real interface.
var _ awsclient.VPCProvisioner = (*VPCProvisioner)(nil)

// NewVPCProvisioner builds an empty fake VPCProvisioner.
func NewVPCProvisioner() *VPCProvisioner {
	return &VPCProvisioner{vpcs: make(map[string]string)}
}

func (f *VPCProvisioner) FindVPCByName(ctx context.Context, name string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.vpcs[name]
	return id, ok, nil
}

func (f *VPCProvisioner) CreateVPC(ctx context.Context, name, cidrBlock string, enableDNSSupport bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.vpcs[name]; ok {
		return "", fmt.Errorf("VPC %q already exists", name)
	}
	f.nextID++
	id := fmt.Sprintf("vpc-fake%08d", f.nextID)
	f.vpcs[name] = id
	return id, nil
}

func (f *VPCProvisioner) DeleteVPC(ctx context.Context, vpcID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, id := range f.vpcs {
		if id == vpcID {
			delete(f.vpcs, name)
			return nil
		}
	}
	return fmt.Errorf("VPC %q does not exist", vpcID)
}
