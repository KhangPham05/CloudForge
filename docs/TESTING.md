# Testing

How we know CloudForge actually works — not just "tests pass," but what each layer of testing can and can't prove, two real bugs caught along the way, and an honest account of a coverage number that went *down* before it went back up.

## Two layers, for two different jobs

- **Hermetic (`envtest`)** — a real `kube-apiserver` + `etcd`, no Docker, no LocalStack. Every controller runs against an in-memory fake `AWSClient`. Fast, fully isolated, runs anywhere (including CI).
- **Live (`-tags=integration`)** — the real `aws-sdk-go-v2` client, against a real LocalStack container. Slower, needs infrastructure running, proves the actual wire-format calls work, not just our own logic's assumptions about them.

`S3Bucket` and `VPC` have both. `RDSInstance` is hermetic-only, by design — see [`DESIGN_DECISIONS.md`](./DESIGN_DECISIONS.md) for why it's backed by a fake even in normal operation, not just in tests.

## A structural limit we designed around, not ignored

`envtest` starts only `kube-apiserver` and `etcd` — it does not run `kube-controller-manager`, which is where Kubernetes's real garbage-collector controller lives. That means a hermetic test can verify an `OwnerReference` is set *correctly* (the right `kind`, `name`, `controller: true`), but it structurally cannot verify that deleting the owner actually cascades the delete — the controller that does that isn't running in that environment at all.

Rather than let that gap go unnoticed, it's documented directly in the test code (`rdsinstance_controller_test.go`), and proven separately, live, against the real `kind` cluster: delete an `RDSInstance`, and its generated credentials `Secret` disappears on its own — `kubectl get secret ... → Error from server (NotFound)` — with zero application code ever calling delete on it.

## Two bugs, caught two different ways

**Caught by reasoning, before a single test ran.** AWS hands back an RDS instance's master password exactly once, in `CreateDBInstance`'s own response — never again. An early draft of the reconcile loop checked whether the database was `Available` *before* persisting that password. Since provisioning is asynchronous by design, that check would have discarded the password on nearly every real run. Found by tracing the data lifecycle by hand before writing the Secret-generation code at all, not by debugging a failure after shipping it.

**Caught by actually running the tests, not by trusting them.** The first version of the Secret-content tests read `secret.StringData["DB_PASSWORD"]` after a `Get()` and got an empty string — a real test failure. Root cause: a Kubernetes `Secret`'s `StringData` field is write-only; the API server merges it into `.Data` (base64-decoded) at write time and never returns it again on read. A *second*, subtler version of the same mistake was hiding in a different test: comparing `StringData` across two reads to prove a Secret wasn't recreated — since both reads would return empty strings, that comparison would have silently "passed" without proving anything at all. Both were fixed to read `.Data` instead, and only caught because the tests were actually executed, not assumed correct from reading the code.

## Coverage went down before it went back up

Worth stating plainly rather than smoothing over: partway through, `internal/controller` coverage dropped from 56.0% to 22.9%. Not a regression in the code — a finalizer was added to the reconcile loop, and the existing test only called `Reconcile()` once, so it stopped reaching the create/status logic it used to exercise, now that the first call just attaches the finalizer. The honest number surfaced the gap immediately; it was fixed by rewriting the test suite to drive the loop through its full real lifecycle (multiple `Reconcile()` calls, a dedicated delete scenario), landing at 60.4% — higher than the original 56.0%, because it now covered strictly more behavior than before.

## Flakiness discipline

`go test` (and `make test`) silently replays a cached result if nothing's changed — looks identical to a real run, proves nothing about *this* run. Before trusting any test suite here, it's re-run with `-count=1` (forces real re-execution) three times fresh, specifically checking for flakiness from shared state between test cases. Every suite in this project has passed that check.

## Real numbers

| | |
|---|---|
| `internal/controller` (hermetic) | **67.0%** coverage, 11/11 specs passing, non-flaky across 3 fresh reruns |
| `internal/awsclient` (`-tags=integration`, real LocalStack) | **62.9%** coverage |
| `internal/awsclient/fake` | **45.1%** coverage |
| Manual AWS CLI steps eliminated | **7 steps → 1 `kubectl apply -k` command** |
| Cold start (kind + LocalStack + CRDs + operator → all 3 `Ready`) | **153 seconds**, measured end-to-end with real timestamps |
