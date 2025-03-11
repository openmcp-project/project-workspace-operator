[![REUSE status](https://api.reuse.software/badge/github.com/openmcp-project/project-workspace-operator)](https://api.reuse.software/info/github.com/openmcp-project/project-workspace-operator)

# project-workspace-operator

## About this project

This repository contains the controllers which reconcile Project and Workspace resources, create namespaces for them, and handle permissions. It is part of the onboarding system.

## Requirements and Setup

### Prerequisites
In order to run the operator locally, you need to have the following tools installed:
- [Go](https://golang.org/dl/)
- [Docker](https://docs.docker.com/get-docker/)
- [Kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- [KIND](https://kind.sigs.k8s.io/)

### Running the Operator Locally
1. Execute `make dev-local` to create a local KIND cluster and deploy the operator via Helm.
2. Look at the [samples](./config/samples/) directory for examples on how to create Project and Workspace resources. Change the `spec` of the resources to you need.
3. Apply them at the local KIND cluster.

### Generating CRDs and Documentation
For generating the CRDs, DeepCopy methods and documentation, execute the following command:
```shell
make generate
```

### Cleaning up the KIND cluster
To clean up the KIND cluster with the deployed operator and its resources, execute the following command:
```shell
make dev-clean
```

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/openmcp-project/project-workspace-operator/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Security / Disclosure
If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/openmcp-project/project-workspace-operator/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2025 SAP SE or an SAP affiliate company and project-workspace-operator contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/openmcp-project/project-workspace-operator).
