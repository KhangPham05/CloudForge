# CloudForge — Architecture

Technical design detail supporting `../PLAN.md`. This is the reference we'll build against phase by phase.

## Repo layout (target — most of this doesn't exist yet)

```
CloudForge/
├── go.mod                                  # module github.com/KhangPham05/cloudforge
├── PROJECT                                 # kubebuilder project marker (auto-generated)
├── main.go                                 # manager entrypoint, registers all 3 controllers
├── Makefile                                # kubebuilder-generated + custom kind/localstack targets
├── kind-config.yaml                        # kind cluster definition
├── docker-compose.localstack.yaml          # LocalStack container
├── api/v1alpha1/
│   ├── groupversion_info.go                # scheme registration (generated)
│   ├── vpc_types.go                        # VPC, VPCSpec, VPCStatus
│   ├── rdsinstance_types.go                # RDSInstance, RDSInstanceSpec, RDSInstanceStatus
│   ├── s3bucket_types.go                   # S3Bucket, S3BucketSpec, S3BucketStatus
│   └── zz_generated.deepcopy.go            # controller-gen output
├── internal/
│   ├── controller/
│   │   ├── vpc_controller.go
│   │   ├── vpc_controller_test.go          # envtest
│   │   ├── s3bucket_controller.go
│   │   ├── s3bucket_controller_test.go
│   │   ├── rdsinstance_controller.go
│   │   ├── rdsinstance_controller_test.go
│   │   └── suite_test.go                   # envtest bootstrap (generated, shared)
│   └── awsclient/
│       ├── config.go                       # shared aws.Config builder w/ LocalStack endpoint override
│       ├── ec2.go                          # VPCProvisioner interface + real impl
│       ├── s3.go                           # S3Provisioner interface + real impl
│       ├── rds.go                          # RDSProvisioner interface + real impl
│       └── fake/
│           ├── fake_ec2.go
│           ├── fake_s3.go
│           └── fake_rds.go                 # default RDS backend
├── config/                                 # kubebuilder-generated: crd/, rbac/, manager/, samples/
│   └── samples/
│       ├── infra_v1alpha1_vpc.yaml
│       ├── infra_v1alpha1_rdsinstance.yaml
│       └── infra_v1alpha1_s3bucket.yaml
├── test/integration/                       # build-tagged tests against a live LocalStack container
│   ├── s3_integration_test.go
│   └── vpc_integration_test.go
├── docs/architecture.md                    # this file
└── README.md
```

## Why `internal/awsclient` is interface-based

One narrow Go interface per resource — e.g. `VPCProvisioner` with a method like `EnsureVPC(ctx, spec) (vpcID string, err error)`. Each controller's `Reconciler` struct holds one of these as a field: the real aws-sdk-go-v2 implementation in `main.go`, a fake in-memory implementation in tests (and, for RDS, in normal local dev too — see the RDS strategy note below).

This is a clean Go lesson in **interfaces + dependency injection**: the controller code only ever talks to the interface, never the concrete SDK type, so swapping implementations requires no changes to reconcile logic.

`internal/awsclient/config.go` is the shared seam between local dev and real AWS:
```go
func NewAWSConfig(ctx context.Context) (aws.Config, error) {
    endpoint := os.Getenv("AWS_ENDPOINT_URL") // e.g. http://localhost:4566, empty in prod
    optFns := []func(*config.LoadOptions) error{}
    if endpoint != "" {
        optFns = append(optFns, config.WithBaseEndpoint(endpoint))
    }
    cfg, err := config.LoadDefaultConfig(ctx, optFns...)
    if err != nil {
        return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
    }
    return cfg, nil
}
```
This is also the first natural `if err != nil` site worth pausing on.

## CRD field design

### VPC (`api/v1alpha1/vpc_types.go`)
```go
type VPCSpec struct {
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    VPCName string `json:"vpcName"`

    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$`
    CidrBlock string `json:"cidrBlock"`

    // +kubebuilder:default=true
    // +optional
    EnableDnsSupport *bool `json:"enableDnsSupport,omitempty"`
}

type VPCStatus struct {
    // +optional
    VpcId string `json:"vpcId,omitempty"`

    // +kubebuilder:validation:Enum=Pending;Ready
    // +kubebuilder:default=Pending
    Phase string `json:"phase,omitempty"`

    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
```
`EnableDnsSupport *bool` is a pointer because Go's `bool` zero value (`false`) is indistinguishable from "the user explicitly set it to false" — a pointer lets us tell "unset" apart from "explicitly false". `VpcId`/`Phase` stay plain `string`: they're always controller-written, never ambiguous.

Add `+kubebuilder:subresource:status` on the `VPC` type, plus `+kubebuilder:printcolumn` markers for `Phase`/`VpcId` so `kubectl get vpc` is readable.

### RDSInstance (`api/v1alpha1/rdsinstance_types.go`)
```go
type RDSInstanceSpec struct {
    // +kubebuilder:validation:Required
    InstanceClass string `json:"instanceClass"` // e.g. db.t3.micro

    // +kubebuilder:validation:Enum=postgres;mysql
    Engine string `json:"engine"`

    // +kubebuilder:validation:Minimum=20
    AllocatedStorage int32 `json:"allocatedStorage"`

    // +kubebuilder:validation:Required
    VpcRef string `json:"vpcRef"` // name of the VPC CR (same namespace) to provision inside
}

type RDSInstanceStatus struct {
    // +optional
    Endpoint string `json:"endpoint,omitempty"`
    // +optional
    Port int32 `json:"port,omitempty"`

    // +kubebuilder:validation:Enum=Provisioning;Ready
    // +kubebuilder:default=Provisioning
    Phase string `json:"phase,omitempty"`

    // +optional
    SecretRef string `json:"secretRef,omitempty"` // name of generated credentials Secret

    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
```
`VpcRef` is a plain `string` for v1alpha1 (not a structured `ObjectReference`) — deliberately simple for now; a documented future improvement, not built yet.

### S3Bucket (`api/v1alpha1/s3bucket_types.go`)
```go
type S3BucketSpec struct {
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`
    BucketName string `json:"bucketName"`

    // +kubebuilder:validation:Enum=private;public-read
    // +kubebuilder:default=private
    Acl string `json:"acl,omitempty"`

    // +kubebuilder:default=false
    // +optional
    VersioningEnabled *bool `json:"versioningEnabled,omitempty"`
}

type S3BucketStatus struct {
    // +optional
    Arn string `json:"arn,omitempty"`

    // +kubebuilder:validation:Enum=Provisioning;Ready
    // +kubebuilder:default=Provisioning
    Phase string `json:"phase,omitempty"`

    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
```
Same `*bool` pattern for `VersioningEnabled` — deliberate repetition of the pointer-for-optional-field lesson.

## Reconcile loop design

Shared skeleton across all three controllers:

1. `Get` the CR. `apierrors.IsNotFound(err)` → return `ctrl.Result{}, nil` (object's gone, nothing to do). Any other `err != nil` → return `ctrl.Result{}, err` (triggers client-go's backoff-requeue).
2. If `!obj.DeletionTimestamp.IsZero()`: object is being deleted — run finalizer cleanup, then `controllerutil.RemoveFinalizer` + `Update`, return.
3. Else if the finalizer isn't present yet: `controllerutil.AddFinalizer` + `Update`, return early (this re-triggers reconcile).
4. Check-exists-before-create against AWS (`Describe*`/`Head*` first, `Create*` only if not found) — this is what makes reconciliation idempotent across replays.
5. Update `.Status` via `r.Status().Update(ctx, obj)` (the status subresource, not the main object) and set conditions with `apimeta.SetStatusCondition`.
6. Return `ctrl.Result{RequeueAfter: X}` while waiting on something async; `ctrl.Result{}, err` on transient errors (so backoff applies); `ctrl.Result{}, nil` once steady-state Ready.

### VPC controller
- Finalizer: `infra.cloudforge.dev/vpc-finalizer`.
- Create path: describe by tag → if absent, `CreateVpc(cidr, name)`, each SDK call wrapped as `if err != nil { return ctrl.Result{}, fmt.Errorf("creating vpc: %w", err) }`.
- Set `Status.VpcId`, `Status.Phase = "Ready"`, Condition `Ready=True`.
- Delete path: before `DeleteVpc`, list `RDSInstance` CRs and check none still reference this VPC (`Spec.VpcRef == vpc.Name`) — if any do, requeue instead of deleting, to respect dependency order.

### S3Bucket controller (build this first — the "quick win")
- Finalizer: `infra.cloudforge.dev/s3bucket-finalizer`.
- Create path: `HeadBucket` — distinguish "NotFound" (proceed to create) from other errors via a `smithy.APIError` type-assertion. `CreateBucket`, then `PutBucketAcl` if non-default, `PutBucketVersioning` if enabled.
- Set `Status.Arn`, `Status.Phase = "Ready"`.
- Delete path: list and delete all objects in the bucket first, then `DeleteBucket` — avoids a stuck finalizer if the bucket was ever written to.

### RDSInstance controller (cross-resource lookup + Secret handoff)
- Finalizer: `infra.cloudforge.dev/rdsinstance-finalizer`.
- Cross-resource lookup: `r.Get(ctx, client.ObjectKey{Namespace: rds.Namespace, Name: rds.Spec.VpcRef}, &vpc)` — NotFound → Condition `VPCNotFound`, `RequeueAfter: 10s`; other errors → return err.
- Wait-for-Ready: `if vpc.Status.Phase != "Ready" { set Condition WaitingOnVPC; return ctrl.Result{RequeueAfter: 15*time.Second}, nil }` — this is *not* an error, just "not ready yet."
- Once ready: check-exists via `DescribeDBInstances`, else `CreateDBInstance`; poll via `RequeueAfter` with `Status.Phase = "Provisioning"` until available.
- **Secret generation**: build a `corev1.Secret` named `<rds-cr-name>-credentials` with `DB_HOST`/`DB_PORT`/`DB_PASSWORD` (the master password is only available at creation time, so it must be captured then). `controllerutil.SetControllerReference(rds, secret, r.Scheme)` gives owner-reference-based garbage collection. `r.Create`, handling `apierrors.IsAlreadyExists`. Store `Status.SecretRef`.
- Delete path: `DeleteDBInstance` (skip final snapshot — fine for a demo).

## RDS strategy: fake provisioner by default

LocalStack's free Hobby tier no longer includes RDS (Base tier is $39/mo, or a time-boxed Ultimate trial). We build RDS against `internal/awsclient/fake/fake_rds.go` by default — an in-memory implementation of `RDSProvisioner`. Full reconcile logic (cross-resource VPC waiting, provisioning-state polling, Secret+OwnerReference generation) gets built and fully tested via envtest, at zero cost, via the same interface-swap pattern used for VPC/S3. VPC and S3 controllers run against real LocalStack (both still free).

The README will state this plainly as a deliberate, testability-driven tradeoff rather than hide it.

## Local dev stack

- **kind**: single control-plane node. The operator runs on the *host* via `make run` (not in-cluster), so no special networking is needed — both kind's API server and LocalStack are reachable at `localhost`.
- **`docker-compose.localstack.yaml`**:
```yaml
services:
  localstack:
    image: localstack/localstack:latest
    ports:
      - "4566:4566"
    environment:
      - SERVICES=s3,ec2
      - DEBUG=1
```
(RDS intentionally left out of `SERVICES` — fake provisioner is used instead.)
- **Makefile additions**: `make kind-up` / `make kind-down`, `make localstack-up` / `make localstack-down`, `make dev-up` (composite: kind-up + localstack-up + install CRDs). `make run` needs `AWS_ENDPOINT_URL=http://localhost:4566 AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=us-east-1` (LocalStack accepts dummy static creds).

## Testing approach

- **envtest** (`internal/controller/suite_test.go`, kubebuilder-generated): a real `kube-apiserver` + `etcd`, no Docker/LocalStack needed. Inject fake `awsclient` implementations for fast, hermetic tests — finalizer add/remove, status writes, owner-reference GC, and the full RDS wait-for-VPC-Ready state machine.
- **Integration tests** (`test/integration/*_test.go`, `//go:build integration`): run against a live LocalStack container using the *real* `awsclient` implementations for S3 and VPC, to prove actual round-trips through the AWS SDK v2 wire format. No RDS integration test (fake-only).
- No testing against real AWS — not worth the cost/credential risk for a portfolio project.

## README / demonstration plan

- **Architecture diagram**: three controllers in one manager binary, each holding a `client.Client` (→ kind API server) and an `awsclient.*Provisioner` (→ LocalStack or fake). Arrow from RDSInstance → VPC box labeled "reads Status.Phase via Get()". Dashed OwnerReference arrow from RDSInstance → generated Secret.
- **One-command demo**: `make dev-up` → apply VPC sample → `kubectl get vpc -w` (Pending→Ready) → apply RDSInstance sample (references VPC by name) → `kubectl get rdsinstance -w` (WaitingOnVPC → Provisioning → Ready) → `kubectl get secret -o yaml` showing `ownerReferences`.
- **S3 finalizer demo**: apply S3Bucket sample, confirm bucket exists via `aws --endpoint-url=http://localhost:4566 s3 ls`, `kubectl delete s3bucket <name>`, show the finalizer briefly holding the object, confirm bucket and CR both disappear — a good terminal recording/gif candidate.
- Call out the LocalStack RDS tiering decision near the RDS section of the README, framed as a deliberate engineering tradeoff.
