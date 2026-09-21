# CloudForge — Plan

## Context

CloudForge is a Kubernetes operator, written in Go with kubebuilder/controller-runtime, that manages the lifecycle of AWS infrastructure declaratively through three CRDs. It's built as a portfolio project for a DGX Cloud Kubernetes Runtime internship application — the goal is to demonstrate real operator skills (reconciliation, finalizers, cross-resource dependencies, credential handoff), not a toy CRD.

It's also a Go-learning project. Along the way we're calling out core Go concepts explicitly: structs & tags, pointers (`*T`) and why Go uses them, and Go's explicit `if err != nil` error handling. See `docs/architecture.md` for where each of these shows up in the code.

Development and testing is entirely free: **kind** (Kubernetes in Docker) for the cluster, **LocalStack** (Docker) for AWS emulation, the operator running on the host via `make run`.

We're building this **incrementally with discussion at each step** — implement a small piece, explain it, get approval, move on. This file tracks the big picture; `docs/architecture.md` has the technical design detail.

## Naming decisions

- Domain: `cloudforge.dev`
- API group/version: `infra.cloudforge.dev/v1alpha1`
- Go module: `github.com/KhangPham05/cloudforge`

## Scope: 3 CRDs, deliberately not more

VPC, RDSInstance, S3Bucket — and that's it. EC2 and IAM were considered and explicitly cut: EC2 (standalone) would repeat the VPC pattern without teaching anything new (it only gets interesting paired with ALB/Auto Scaling, which is a different, bigger project); IAM is real complexity (trust policies, attached policies) that mostly demonstrates more SDK surface area, not a new operator concept. Three CRDs built deep (real tests, real finalizers, real cross-resource logic) reads stronger to a reviewer than five built shallow. A "Future Work" note in the README will name EC2/IAM as where the pattern generalizes, without expanding actual scope.

## The three CRDs (one-line each — full field design in `docs/architecture.md`)

1. **VPC** — network layer, other resources depend on it. Spec: name, CIDR, DNS support. Status: VpcId, Phase, Conditions.
2. **RDSInstance** — data layer, references a VPC by name and waits for it to be Ready before provisioning. Status includes a generated credentials Secret reference.
3. **S3Bucket** — storage layer, standalone. Demonstrates finalizer-based cleanup on delete.

## Key architecture decisions

- **AWS access is interface-based** (`internal/awsclient`): one Go interface per resource (`VPCProvisioner`, `S3Provisioner`, `RDSProvisioner`), each with a real aws-sdk-go-v2 implementation and a fake in-memory implementation. Controllers depend on the interface, not the concrete client — real for local/prod, fake for fast hermetic tests.
- **RDS uses the fake provisioner by default.** LocalStack dropped RDS from its free Hobby tier (now requires a paid Base tier, $39/mo, or a time-boxed trial). VPC and S3 stay on real LocalStack (both still free). This is a deliberate, documented tradeoff — not a shortcut we're hiding.
- **S3 finalizer empties the bucket before deleting it** (S3 refuses to delete non-empty buckets), rather than assuming buckets stay empty.

## Why this stands out (the hiring-signal map)

Every non-obvious technical decision here was made deliberately to *demonstrate* something, not just to make the demo work. This table is the "why should a reviewer care" translation layer — keep it in mind while building, since it's what turns a working repo into a story worth telling:

| Decision | Skill it proves | Why a reviewer cares |
|---|---|---|
| 3 CRDs + controller-runtime/kubebuilder | You understand the Kubernetes *extension* model, not just consuming built-in resources | This is literally what a Kubernetes Runtime team builds and maintains |
| Idempotent reconcile loop (check-before-create, drift-aware) | You understand eventual consistency and self-healing systems, not one-shot scripts | Production controllers must survive restarts, races, replayed events |
| Finalizers for cleanup | You understand Kubernetes GC semantics, not just "delete and hope" | Prevents orphaned cloud resources — a real cost/security problem at scale |
| Cross-resource choreography (RDS waits on VPC via `Get()` + requeue) | You can sequence multi-resource dependencies without an external workflow engine | Mirrors how real platform teams order infra provisioning |
| Interface-based AWS client (real SDK + fake) | You design for testability and cost-consciousness as a deliberate tradeoff, not an afterthought | Reads as senior instinct even from an intern-level candidate |
| Owner-referenced Secret auto-generation | You understand secure credential handoff patterns | Directly maps to "cloud control plane" security concerns |
| LocalStack + kind, zero cloud spend | You can build and validate complex infra without burning money | Resourcefulness + cost discipline, not just "I can follow a tutorial" |

This is also where the resume bullets and interview answers come from — each row above is a "tell me about a time you..." answer waiting to happen, not just a feature.

## Build order (14 phases)

Each phase is scoped to ~1-2 new concepts and ends with something concrete to look at (a command output, a `kubectl get`, a passing test) — a natural stop-and-discuss point before moving on.

0. **Tooling setup** — install `kind` + `kubebuilder`, verify versions.
1. **Project scaffold** — `kubebuilder init`. Look at what it generated (`go.mod`, `PROJECT`, `main.go`, `Makefile`) before writing any of our own code.
2. **Local infra up** — kind cluster + LocalStack via docker-compose, running side by side. Prove both are reachable (`kubectl cluster-info`, LocalStack health check) — no operator code yet.
3. **AWS client foundation** — `internal/awsclient/config.go` (LocalStack endpoint override) plus the *empty* interfaces (`VPCProvisioner`, `S3Provisioner`, `RDSProvisioner`) — contracts only, no implementations. The Go interfaces lesson, isolated.
4. **First CRD: S3Bucket type** — `kubebuilder create api --kind S3Bucket`, define `S3BucketSpec`/`Status` structs with json tags + kubebuilder markers. Install the CRD, `kubectl get s3bucket` — prove the schema round-trips through the API server, controller does nothing yet.
5. **S3 AWS client implementation** — fill in `internal/awsclient/s3.go` (real SDK calls: CreateBucket/HeadBucket/DeleteBucket/PutBucketVersioning) against LocalStack, verified standalone before wiring into a controller.
6. **S3 Reconcile: create path** — fetch CR, check-exists, create bucket, update `Status.Phase = Ready`. No finalizer yet. Demo: apply a sample, watch it go Ready.
7. **S3 Reconcile: finalizer + delete path** — register the finalizer, empty-then-delete cleanup. Demo: `kubectl delete`, watch the bucket actually disappear from LocalStack.
8. **S3 tests** — `fake_s3.go` + envtest suite for the S3 controller. Learn the hermetic-testing pattern once here before repeating it.
9. **VPC CRD (types → client → reconcile → finalizer)** — repeat the same pattern from phases 4-7 for VPC, faster the second time through.
10. **RDS CRD types + fake provisioner** — define `RDSInstanceSpec`/`Status`, build `fake_rds.go`. No reconcile logic yet.
11. **RDS cross-resource choreography** — the new concept: `Get()` the referenced VPC, wait/requeue until it's Ready.
12. **RDS provisioning + Secret handoff** — create the "DB" via the fake provisioner, capture credentials, generate the owner-referenced Kubernetes Secret.
13. **Integration tests, README, and real numbers** — `test/integration` against live LocalStack, write the README, and capture the actual metrics (manual-steps count, cold-start time, coverage %) to finalize the resume bullets.

## Resume bullets — metric-driven, filled in as we build

Rule: no invented numbers. Every bullet below gets a real measurement captured at the point in the build where it becomes possible — never backfilled from a guess. Format stays `Accomplished [X] by doing [Y] using [Z], resulting in [measured impact]`.

1. **Cost avoidance (known now):** Accomplished zero-cost RDS reconciliation testing by architecting an interface-based AWS client (real aws-sdk-go-v2 vs. in-memory fake) using Go, resulting in **$468/year avoided** vs. LocalStack's paid RDS tier ($39/mo Base), while still validating VPC and S3 against real LocalStack emulation.
2. **Manual-steps eliminated (measure at end of Phase 4):** Accomplished one-command AWS infrastructure provisioning by building a custom Kubernetes operator with three CRDs using kubebuilder/controller-runtime, resulting in **[N manual AWS console/CLI steps] → 1 `kubectl apply` command**. *(To measure: walk through creating a VPC + RDS instance + S3 bucket by hand via AWS CLI/console, count the steps.)*
3. **Setup time (measure at end of Phase 4):** Accomplished automated environment provisioning by scripting `make dev-up` (kind + LocalStack + CRDs + operator), resulting in **infrastructure Ready in [X seconds/minutes] from a cold start**. *(To measure: time a clean run from empty cluster to all 3 sample CRs showing `Phase: Ready`.)*
4. **Test coverage & reliability (measure after envtest suite is written):** Accomplished verified reconciliation correctness by writing an envtest suite covering create/update/delete/drift paths for all 3 controllers, resulting in **[X]% statement coverage across [N] test cases, 0 flakes across [M] runs**. *(To measure: `go test -cover ./internal/controller/...`, plus re-running the suite a few times to confirm no flakiness.)*

We'll also keep the credential-handoff story (RDS → auto-generated Secret) as a *qualitative* bullet if no clean number attaches to it — not every accomplishment needs a metric forced onto it; a clear before/after ("manual secret creation → automatic, owner-referenced Secret generation") is legitimate on its own.

## Verification (once built)

- `make dev-up` → kind + LocalStack up, CRDs installed, `make run` starts the operator on the host.
- Apply samples in order (VPC → S3 → RDS), watch `kubectl get <kind> -w` transition through Phase values to Ready.
- `go test ./internal/controller/...` — envtest suite (hermetic, fake AWS clients).
- `go test -tags=integration ./test/integration/...` — real-LocalStack round-trip tests for VPC/S3.
- Manual demo: S3 finalizer cleanup, RDS Secret owner-reference — both described in `docs/architecture.md`.

## Open items

- Deferred (not blocking): a structured `VpcRef` type (v1alpha2) instead of a bare string — noted as a future improvement, intentionally out of scope now.
