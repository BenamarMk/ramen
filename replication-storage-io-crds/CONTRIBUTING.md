# Contributing to Replication Storage API

Thank you for your interest in contributing to the Kubernetes Replication Storage API!

## Code of Conduct

This project follows the [Kubernetes Code of Conduct](https://kubernetes.io/community/code-of-conduct/).

## How to Contribute

### Reporting Issues

- Use GitHub Issues to report bugs or request features
- Search existing issues before creating a new one
- Provide detailed information including:
  - Kubernetes version
  - Storage provider
  - Steps to reproduce
  - Expected vs actual behavior

### Proposing Changes

1. **Fork the repository**
2. **Create a feature branch** (`git checkout -b feature/my-feature`)
3. **Make your changes**
4. **Test your changes** (see Testing section)
5. **Commit with clear messages** (`git commit -s -m "Add feature X"`)
6. **Push to your fork** (`git push origin feature/my-feature`)
7. **Open a Pull Request**

### Developer Certificate of Origin

All commits must be signed off (`git commit -s`) to indicate you agree to the [Developer Certificate of Origin](https://developercertificate.org/).

## Development Setup

### Prerequisites

- Go 1.21+
- kubectl
- kustomize
- Access to a Kubernetes cluster (for testing)

### Building

```bash
# Generate CRDs from API types
make generate

# Build install.yaml
make build
```

### Testing

```bash
# Install CRDs in test cluster
kubectl apply -k config/

# Verify installation
kubectl get crds | grep replication.storage.io

# Clean up
kubectl delete -k config/
```

## API Changes

### Modifying CRDs

1. Update the API types in `api/replication.storage.io/v1alpha1/`
2. Run `make generate` to regenerate CRDs
3. Update examples in `examples/`
4. Update documentation in `docs/`
5. Test with real storage providers

### API Compatibility

- **v1alpha1**: Breaking changes allowed with notice
- **v1beta1**: Breaking changes require deprecation period
- **v1**: No breaking changes allowed

## Documentation

- Update README.md for user-facing changes
- Update docs/ for detailed documentation
- Include examples for new features
- Update API reference

## Review Process

1. **Automated checks** must pass (linting, validation)
2. **Maintainer review** required for all PRs
3. **Storage provider testing** for API changes
4. **Documentation review** for user-facing changes

## Release Process

Releases are managed by maintainers:

1. Version bump in appropriate files
2. Update CHANGELOG.md
3. Create GitHub release
4. Update installation instructions

## Getting Help

- **Slack**: [#replication-storage-io](https://kubernetes.slack.com/messages/replication-storage-io)
- **Mailing List**: kubernetes-sig-storage@googlegroups.com
- **Office Hours**: Bi-weekly Thursdays 10:00 AM PT

## Maintainers

See [OWNERS](OWNERS) file for current maintainers.

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.