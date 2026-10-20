# CloudForge

A Kubernetes operator that manages AWS infrastructure declaratively — define a `VPC`, `S3Bucket`, or `RDSInstance` as a Kubernetes object, and a controller reconciles real cloud state to match. Built with [kubebuilder](https://book.kubebuilder.io/) and [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime), developed and tested entirely for free against [kind](https://kind.sigs.k8s.io/) and [LocalStack](https://www.localstack.cloud/).

> **Status: in active development.** `S3Bucket` and `VPC` are fully implemented — create, finalizer-based cleanup, and tests, verified against a real LocalStack container. `RDSInstance` has its types and an in-memory test double defined; its reconcile logic (waiting on a referenced `VPC`, generating a credentials Secret) is still in progress.

## Description

CloudForge extends the Kubernetes API with three custom resources that represent real AWS infrastructure. Each one follows the same pattern: a `Spec` describes the desired state, a controller watches for changes and reconciles reality to match by calling the AWS SDK, and a `Status` reports back what's actually true — the same control-loop pattern Kubernetes itself uses for Pods and Deployments, applied to cloud infrastructure instead of containers.

- **`S3Bucket`** — provisions an S3 bucket; its finalizer empties the bucket (even if objects were added to it) before deleting it.
- **`VPC`** — provisions a VPC with a given CIDR block and DNS settings.
- **`RDSInstance`** *(in progress)* — provisions a database inside a referenced `VPC`, waiting for that VPC to be ready before proceeding, and will hand back credentials as a generated Kubernetes Secret.

All AWS access goes through an interface-based client layer (`internal/awsclient`), so every controller can run against a real AWS SDK client or an in-memory test double. That's what keeps development and testing entirely free: `S3Bucket` and `VPC` are verified against a real [LocalStack](https://www.localstack.cloud/) container, while `RDSInstance` (which LocalStack only supports on a paid tier) is built and tested against the in-memory double instead.

## Architecture

```
kubectl apply  →  Kubernetes API server (kind)  →  controller watches for changes
                                                            │
                                                  Reconcile() reads Spec,
                                                  calls the AWS client,
                                                  writes Status back
                                                            │
                                            internal/awsclient (interface)
                                            ├── real implementation → LocalStack
                                            └── in-memory test double
```

_A diagram of how `S3Bucket`, `VPC`, and `RDSInstance` relate — including the `RDSInstance` → `VPC` dependency — will be added here once all three are complete._

## Project Structure

```
api/v1alpha1/                 # CRD type definitions (Spec/Status structs)
├── s3bucket_types.go
├── vpc_types.go
└── rdsinstance_types.go

internal/
├── awsclient/                # AWS access, behind interfaces
│   ├── s3.go, ec2.go, rds.go       # interfaces + real SDK implementations
│   └── fake/                       # in-memory test doubles
└── controller/                # one reconciler per CRD
    ├── s3bucket_controller.go
    ├── vpc_controller.go
    └── rdsinstance_controller.go

cmd/main.go                    # manager entrypoint, wires everything together
config/samples/                # example YAML for each CRD
kind-config.yaml                # local Kubernetes cluster definition
docker-compose.localstack.yaml  # local AWS emulator
```

## Getting Started

### Prerequisites
- Go 1.24+
- Docker
- [kind](https://kind.sigs.k8s.io/) and [kubebuilder](https://book.kubebuilder.io/)
- A free [LocalStack](https://www.localstack.cloud/) account (required even for free-tier services) — set `LOCALSTACK_AUTH_TOKEN` in a local `.env` file

### Run it locally

```sh
kind create cluster --config kind-config.yaml
docker compose -f docker-compose.localstack.yaml up -d
make install   # registers the CRDs with the cluster

AWS_ENDPOINT_URL=http://localhost:4566 \
AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=us-east-1 \
make run       # starts the operator on the host
```

In another terminal:

```sh
kubectl apply -f config/samples/infra_v1alpha1_s3bucket.yaml
kubectl get s3bucket -w
```

### Tests

```sh
make test                              # hermetic envtest suite - no LocalStack needed
go test -tags=integration ./...        # round-trips real calls against LocalStack
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
