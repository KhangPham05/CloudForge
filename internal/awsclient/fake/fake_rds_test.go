package fake

import (
	"context"
	"testing"
	"time"

	"github.com/KhangPham05/cloudforge/internal/awsclient"
)

func TestRDSProvisioner_AsyncProvisioning(t *testing.T) {
	ctx := context.Background()
	f := NewRDSProvisioner(50 * time.Millisecond)

	createInfo, err := f.CreateDBInstance(ctx, "mydb", "postgres", "db.t3.micro", 20)
	if err != nil {
		t.Fatalf("CreateDBInstance: %v", err)
	}
	if createInfo.State != awsclient.RDSInstanceStateCreating {
		t.Fatalf("expected state %q right after create, got %q", awsclient.RDSInstanceStateCreating, createInfo.State)
	}
	if createInfo.Password == "" {
		t.Fatal("expected CreateDBInstance to return a non-empty password")
	}

	// Immediately after creation, Describe should still say "creating" -
	// the whole point of the delay is that it hasn't flipped yet.
	info, found, err := f.DescribeDBInstance(ctx, "mydb")
	if err != nil {
		t.Fatalf("DescribeDBInstance (immediate): %v", err)
	}
	if !found {
		t.Fatal("expected to find the instance immediately after creation")
	}
	if info.State != awsclient.RDSInstanceStateCreating {
		t.Fatalf("expected state %q immediately after create, got %q", awsclient.RDSInstanceStateCreating, info.State)
	}
	if info.Password != "" {
		t.Fatal("DescribeDBInstance must never return a password")
	}

	time.Sleep(75 * time.Millisecond)

	info, found, err = f.DescribeDBInstance(ctx, "mydb")
	if err != nil {
		t.Fatalf("DescribeDBInstance (after delay): %v", err)
	}
	if !found {
		t.Fatal("expected to find the instance after the delay")
	}
	if info.State != awsclient.RDSInstanceStateAvailable {
		t.Fatalf("expected state %q after the provisioning delay, got %q", awsclient.RDSInstanceStateAvailable, info.State)
	}
	if info.Endpoint == "" || info.Port == 0 {
		t.Fatalf("expected a real endpoint/port once available, got %+v", info)
	}
}

func TestRDSProvisioner_PortVariesByEngine(t *testing.T) {
	ctx := context.Background()
	f := NewRDSProvisioner(0)

	pgInfo, err := f.CreateDBInstance(ctx, "pgdb", "postgres", "db.t3.micro", 20)
	if err != nil {
		t.Fatalf("CreateDBInstance (postgres): %v", err)
	}
	if pgInfo.Port != 5432 {
		t.Fatalf("expected postgres port 5432, got %d", pgInfo.Port)
	}

	myInfo, err := f.CreateDBInstance(ctx, "mydb", "mysql", "db.t3.micro", 20)
	if err != nil {
		t.Fatalf("CreateDBInstance (mysql): %v", err)
	}
	if myInfo.Port != 3306 {
		t.Fatalf("expected mysql port 3306, got %d", myInfo.Port)
	}
}

func TestRDSProvisioner_DeleteAndDuplicateCreate(t *testing.T) {
	ctx := context.Background()
	f := NewRDSProvisioner(0)

	if _, err := f.CreateDBInstance(ctx, "mydb", "postgres", "db.t3.micro", 20); err != nil {
		t.Fatalf("CreateDBInstance: %v", err)
	}

	if _, err := f.CreateDBInstance(ctx, "mydb", "postgres", "db.t3.micro", 20); err == nil {
		t.Fatal("expected creating a duplicate DB instance to fail")
	}

	if err := f.DeleteDBInstance(ctx, "mydb"); err != nil {
		t.Fatalf("DeleteDBInstance: %v", err)
	}

	if _, found, err := f.DescribeDBInstance(ctx, "mydb"); err != nil || found {
		t.Fatalf("expected instance to be gone after delete, found=%v err=%v", found, err)
	}

	if err := f.DeleteDBInstance(ctx, "mydb"); err == nil {
		t.Fatal("expected deleting an already-deleted DB instance to fail")
	}
}
