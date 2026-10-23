# Architecture

How CloudForge is actually built — the reconcile loop pattern shared by all three resources, how the system's pieces talk to each other, and a practical command reference for working with it.

## The shape of the system

![CloudForge system overview — your laptop running the operator, talking to a kind cluster via the Kubernetes API and to LocalStack via the AWS SDK, with RDS backed by an in-memory fake instead](./images/architecture-overview.svg)

`kubectl` only ever talks to the kind cluster. The operator is the only thing that talks to both sides — Kubernetes (as a client, reading/writing CRs) and AWS (via `internal/awsclient`, either the real SDK or an in-memory fake). Nothing in this system lets Kubernetes talk to AWS directly.

RDS is the one exception worth calling out on the diagram itself: it's backed by the fake by design, not by LocalStack — see [`DESIGN_DECISIONS.md`](./DESIGN_DECISIONS.md) for why.

## The reconcile loop, once

Every controller in this project follows the same shape. Rather than repeat it three times, here it is once:

1. **Fetch** the object. If it's gone (`NotFound`), there's nothing to do — return cleanly, not an error.
2. **Deletion check.** If the object has a `deletionTimestamp`, run cleanup (call AWS to delete the real resource), then remove the finalizer so Kubernetes can finish deleting it.
3. **Finalizer attach.** If the finalizer isn't present yet, add it and return — this re-triggers a fresh reconcile, which is what actually does the work.
4. **Check-before-create.** Ask AWS "does this already exist?" before creating anything — this is what makes the loop idempotent: safe to run again, replayed, or re-triggered by an unrelated change, without duplicating work.
5. **Write `Status`**, via the status subresource specifically (`r.Status().Update(...)`), never the main object — `Spec` is user intent, `Status` is the controller's own report, and the two are mechanically kept separate.
6. **Return.** `RequeueAfter` while waiting on something; a real `error` on a genuine failure (triggers automatic backoff); a clean `nil` once settled.

## What's different about each resource

- **`S3Bucket`** — a bucket's name *is* its identity; nothing else to resolve. Its finalizer empties the bucket (even if objects were added after creation) before deleting it, since S3 refuses to delete a non-empty bucket.
- **`VPC`** — a VPC's real identity is an AWS-generated ID, not the name it's tagged with. The controller has to look it up by tag, then use the returned ID for every subsequent operation.
- **`RDSInstance`** — the one with two genuinely new mechanisms:
  - **Cross-resource waiting.** It reads a *different* CRD's status (`VPC.Status.Phase`) before provisioning, requeuing on a timer while it waits rather than treating "not ready yet" as a failure.
  - **Credential handoff.** AWS hands back a database's master password exactly once, at creation — the controller captures it immediately into a Kubernetes `Secret`, owned via `OwnerReference` so Kubernetes garbage-collects it automatically when the `RDSInstance` is deleted.

![Sequence diagram: the RDSInstance controller calls Get(self), then Get() on the referenced VPC using the same Kubernetes client — if VPC.Status.Phase isn't Ready, it requeues after 15 seconds with a nil error; once Ready, it proceeds to provision the database and generate the credentials Secret](./images/rds-vpc-wait-sequence.svg)

## AWS access: one interface, two implementations

Every controller depends on an interface (`S3Provisioner`, `VPCProvisioner`, `RDSProvisioner`), never a concrete AWS SDK type. That's what makes it possible to run the exact same reconcile code against:

- a **real** implementation, calling `aws-sdk-go-v2` against LocalStack (`S3Bucket`, `VPC`), or
- an **in-memory fake**, used for both tests and, deliberately, RDS's normal operation (see `DESIGN_DECISIONS.md`).

The decision of which backend to use lives in exactly one place (`AWS_ENDPOINT_URL`), read once, in `internal/awsclient/config.go`.

## Useful commands

**Bring the local stack up:**
```sh
kind create cluster --config kind-config.yaml
docker compose -f docker-compose.localstack.yaml up -d
make install   # registers all three CRDs with the cluster

AWS_ENDPOINT_URL=http://localhost:4566 \
AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=us-east-1 \
make run       # starts the operator on the host
```

**Apply everything at once, in any order** (the reconcile loop's wait/requeue logic handles dependency ordering on its own — `RDSInstance` can be applied before its `VPC` even exists):
```sh
kubectl apply -k config/samples/
kubectl get vpc,s3bucket,rdsinstance -w
```

**Inspect what actually happened, not just what the controller claims:**
```sh
aws --endpoint-url=http://localhost:4566 s3 ls
aws --endpoint-url=http://localhost:4566 ec2 describe-vpcs --output json
kubectl get secret <rdsinstance-name>-credentials -o jsonpath='{.data}'
```

**Prove the finalizer actually ran, not just that the object disappeared:**
```sh
kubectl delete s3bucket <name> --wait=true --timeout=30s
```
(`--wait` blocks until the finalizer is fully removed — a plain `kubectl delete` returns immediately after setting `deletionTimestamp`, which doesn't prove cleanup ran.)

**Run the tests:**
```sh
make test                                              # hermetic envtest suite, no LocalStack needed
go test -tags=integration -v ./internal/awsclient/...  # real round-trips against live LocalStack
```
