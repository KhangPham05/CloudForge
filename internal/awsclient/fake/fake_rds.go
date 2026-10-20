package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/KhangPham05/cloudforge/internal/awsclient"
)

// RDSProvisioner is an in-memory fake implementing awsclient.RDSProvisioner.
// Unlike the S3 and VPC fakes, this one deliberately simulates real
// RDS's asynchronous provisioning: a database stays in the "creating"
// state for a short delay after creation before DescribeDBInstance
// reports it as "available" - the same shape of behavior the real
// AWS API has, just compressed to seconds instead of minutes.
type RDSProvisioner struct {
	mu                sync.Mutex
	instances         map[string]*fakeRDSInstance
	provisioningDelay time.Duration
}

// fakeRDSInstance deliberately has no password field - once
// CreateDBInstance hands the password back, nothing keeps a copy of
// it, so there's no way for DescribeDBInstance to accidentally leak
// it later even by mistake.
type fakeRDSInstance struct {
	state    awsclient.RDSInstanceState
	endpoint string
	port     int32
	readyAt  time.Time
}

// compile-time check that RDSProvisioner satisfies the real interface.
var _ awsclient.RDSProvisioner = (*RDSProvisioner)(nil)

// NewRDSProvisioner builds an empty fake RDSProvisioner.
// provisioningDelay controls how long a database stays in the
// "creating" state after CreateDBInstance - pass 0 for tests that
// want instant availability, or a few seconds for a live demo that
// should visibly show a Provisioning -> Ready transition.
func NewRDSProvisioner(provisioningDelay time.Duration) *RDSProvisioner {
	return &RDSProvisioner{
		instances:         make(map[string]*fakeRDSInstance),
		provisioningDelay: provisioningDelay,
	}
}

func (f *RDSProvisioner) DescribeDBInstance(ctx context.Context, name string) (awsclient.RDSInstanceInfo, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	inst, ok := f.instances[name]
	if !ok {
		return awsclient.RDSInstanceInfo{}, false, nil
	}

	if inst.state == awsclient.RDSInstanceStateCreating && !time.Now().Before(inst.readyAt) {
		inst.state = awsclient.RDSInstanceStateAvailable
	}

	return awsclient.RDSInstanceInfo{
		State:    inst.state,
		Endpoint: inst.endpoint,
		Port:     inst.port,
	}, true, nil
}

func (f *RDSProvisioner) CreateDBInstance(ctx context.Context, name, engine, instanceClass string, allocatedStorage int32) (awsclient.RDSInstanceInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.instances[name]; ok {
		return awsclient.RDSInstanceInfo{}, fmt.Errorf("DB instance %q already exists", name)
	}

	port := int32(5432)
	if engine == "mysql" {
		port = 3306
	}
	endpoint := fmt.Sprintf("%s.fake-rds.localhost", name)

	f.instances[name] = &fakeRDSInstance{
		state:    awsclient.RDSInstanceStateCreating,
		endpoint: endpoint,
		port:     port,
		readyAt:  time.Now().Add(f.provisioningDelay),
	}

	return awsclient.RDSInstanceInfo{
		State:    awsclient.RDSInstanceStateCreating,
		Endpoint: endpoint,
		Port:     port,
		Password: "fake-generated-password-" + name,
	}, nil
}

func (f *RDSProvisioner) DeleteDBInstance(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.instances[name]; !ok {
		return fmt.Errorf("DB instance %q does not exist", name)
	}
	delete(f.instances, name)
	return nil
}
