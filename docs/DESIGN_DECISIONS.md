# Design Decisions

Why CloudForge is built the way it is — the trade-offs, scope cuts, and judgment calls behind it. For *how* it works, see [`ARCHITECTURE.md`](./ARCHITECTURE.md); for *how we know it's correct*, see [`TESTING.md`](./TESTING.md).

## Scope: three CRDs, deliberately not more

`VPC`, `S3Bucket`, `RDSInstance` — and that's it. `EC2` and `IAM` were considered and cut on purpose:

- **EC2** (a standalone instance) would repeat the exact same pattern as `VPC` without teaching anything new. It only gets genuinely interesting paired with an Auto Scaling Group and a Load Balancer — a different, bigger project.
- **IAM** (roles, trust policies, attached policies) is real complexity, but it's mostly *more AWS SDK surface area*. It doesn't introduce a new reconciliation concept the way cross-resource waiting or credential handoff do.

Three resources built deep — real finalizers, real cross-resource logic, real tests — reads stronger than five built shallow. The pattern generalizes cleanly to EC2/IAM if it's ever needed; building them now wouldn't prove anything new.

## RDS is backed by an in-memory fake, not real AWS — on purpose

LocalStack's free tier used to include RDS. It doesn't anymore: RDS now requires a paid Base tier ($39/mo) or a time-boxed trial — confirmed directly against LocalStack's own pricing docs before committing to this, not assumed.

Rather than pay for it, the AWS-access layer is interface-based from the start. Every controller depends on an interface (`S3Provisioner`, `VPCProvisioner`, `RDSProvisioner`), never a concrete SDK type:

- `S3Bucket` and `VPC` run their real implementation against LocalStack (both genuinely free).
- `RDSInstance` runs its fake implementation everywhere — tests *and* normal local operation.

The fake isn't a simplified stand-in, either. It genuinely simulates RDS's asynchronous provisioning — a database stays in a `creating` state for a real, configurable delay before becoming `available` — because that's precisely the behavior the reconcile loop's polling logic needs to be exercised against.

**The number this produces:** $468/year avoided (12 × $39/mo) vs. paying for LocalStack's RDS tier, while still validating `VPC` and `S3Bucket` against real infrastructure. Not an estimate — the actual LocalStack Base tier price at the time this decision was made.

## Interfaces grow on demand, not upfront

Nothing in this project tried to predict every method an interface would eventually need:

- **`S3Provisioner.EmptyBucket`** didn't exist until the finalizer/delete logic needed it — S3 refuses to delete a non-empty bucket, so the finalizer has to empty it first. Adding the method only once it was needed kept the interface honest: every method on it corresponds to something a caller actually does.
- **`VPCProvisioner.CreateVPC`** originally had no way to pass `EnableDnsSupport` through, even though the CRD's own spec exposes that field. A real gap, found while implementing the real client, not predicted in advance.

Both times, the fix was the same shape: extend the interface when something real needs it, not before.

A compile-time check (`var _ Interface = (*impl)(nil)`) on every fake implementation makes this safe to do. The moment an interface grows, every implementation that hasn't kept up fails to build — immediately, not as a confusing runtime surprise later.

## Small decisions, stated plainly

- **The S3 finalizer empties the bucket before deleting it**, rather than assuming demo buckets stay empty. S3 itself refuses to delete a non-empty bucket, so this isn't optional if the finalizer is going to work on a bucket that was ever actually used.
- **`RDSInstance.Spec.VpcRef` is a plain string**, not a structured reference type, for now — deliberately simple for a single-namespace project. A documented, intentionally-deferred improvement, not an oversight.
- **A VPC's identity is treated as its AWS-generated ID, not its name**, everywhere in the code that matters (status fields, delete calls). That's genuinely how AWS treats it; the name is just a tag a human (or our own lookup code) uses to find it.
