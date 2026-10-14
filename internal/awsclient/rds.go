package awsclient

import "context"

// RDSInstanceState is the lifecycle state of a database instance, as
// reported back by AWS (or, by default, our fake implementation).
type RDSInstanceState string

const (
	RDSInstanceStateCreating  RDSInstanceState = "creating"
	RDSInstanceStateAvailable RDSInstanceState = "available"
)

// RDSInstanceInfo is what we know about a database instance once it
// exists.
type RDSInstanceInfo struct {
	State    RDSInstanceState
	Endpoint string
	Port     int32

	// Password is only ever populated by CreateDBInstance's return
	// value, at the moment of creation. AWS never hands the master
	// password back again after that, so DescribeDBInstance always
	// leaves this field empty.
	Password string
}

// RDSProvisioner is everything the RDSInstance controller needs to
// manage a database's lifecycle.
type RDSProvisioner interface {
	// DescribeDBInstance looks up an existing database by name. found
	// is false if it hasn't been created yet.
	DescribeDBInstance(ctx context.Context, name string) (info RDSInstanceInfo, found bool, err error)

	// CreateDBInstance starts creating a new database. Creation is
	// asynchronous: the returned info reflects the instance's initial
	// "creating" state, not its final "available" state. The
	// controller must call DescribeDBInstance again later to find out
	// when it becomes available.
	CreateDBInstance(ctx context.Context, name, engine, instanceClass string, allocatedStorage int32) (info RDSInstanceInfo, err error)

	// DeleteDBInstance deletes a database instance by name.
	DeleteDBInstance(ctx context.Context, name string) error
}
