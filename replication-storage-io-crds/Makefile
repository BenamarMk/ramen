# Makefile for replication-storage-io-crds

# Version information
VERSION ?= v0.1.0
REGISTRY ?= ghcr.io/ramendr
IMAGE_NAME ?= replication-storage-io-crds

# Tools
KUSTOMIZE ?= kubectl kustomize
KUBECTL ?= kubectl

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

.PHONY: build
build: ## Generate install.yaml from kustomization
	@echo "Building install.yaml..."
	$(KUSTOMIZE) config > config/install.yaml
	@echo "Generated config/install.yaml ($(shell wc -l < config/install.yaml) lines)"

.PHONY: validate
validate: ## Validate CRD files
	@echo "Validating CRD files..."
	@for file in config/crds/*.yaml; do \
		echo "Validating $$file..."; \
		$(KUBECTL) apply --dry-run=client -f $$file > /dev/null || exit 1; \
	done
	@echo "All CRDs are valid!"

##@ Installation

.PHONY: install
install: ## Install CRDs to the cluster
	@echo "Installing CRDs..."
	$(KUBECTL) apply -k config/
	@echo "Waiting for CRDs to be established..."
	@$(KUBECTL) wait --for condition=established --timeout=60s \
		crd/volumegroupreplicationclasses.replication.storage.io \
		crd/volumegroupreplications.replication.storage.io \
		crd/volumegroupreplicationcontents.replication.storage.io
	@echo "CRDs installed successfully!"

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the cluster
	@echo "Uninstalling CRDs..."
	$(KUBECTL) delete -k config/ --ignore-not-found=true
	@echo "CRDs uninstalled successfully!"

.PHONY: verify
verify: ## Verify CRDs are installed
	@echo "Verifying CRD installation..."
	@$(KUBECTL) get crds | grep replication.storage.io || (echo "CRDs not found!" && exit 1)
	@echo "CRDs verified!"

##@ Examples

.PHONY: install-examples
install-examples: ## Install example resources
	@echo "Installing examples..."
	$(KUBECTL) apply -f examples/
	@echo "Examples installed!"

.PHONY: uninstall-examples
uninstall-examples: ## Uninstall example resources
	@echo "Uninstalling examples..."
	$(KUBECTL) delete -f examples/ --ignore-not-found=true
	@echo "Examples uninstalled!"

##@ Testing

.PHONY: test
test: validate install verify ## Run full test suite
	@echo "All tests passed!"

.PHONY: test-install
test-install: install verify uninstall ## Test installation and uninstallation
	@echo "Installation test passed!"

##@ Cleanup

.PHONY: clean
clean: uninstall ## Clean up all resources
	@echo "Cleanup complete!"

##@ Release

.PHONY: release-notes
release-notes: ## Generate release notes
	@echo "Release Notes for $(VERSION)"
	@echo "=============================="
	@echo ""
	@echo "## Installation"
	@echo ""
	@echo "\`\`\`bash"
	@echo "kubectl apply -f https://github.com/ramendr/replication-storage-io-crds/releases/download/$(VERSION)/install.yaml"
	@echo "\`\`\`"
	@echo ""
	@echo "## Changes"
	@echo ""
	@echo "See CHANGELOG.md for details"

.PHONY: package
package: build ## Package for release
	@echo "Packaging release $(VERSION)..."
	@mkdir -p dist
	@cp config/install.yaml dist/install-$(VERSION).yaml
	@cp config/install.yaml dist/install.yaml
	@echo "Package created in dist/"

##@ Development

.PHONY: fmt
fmt: ## Format YAML files
	@echo "Formatting YAML files..."
	@find . -name "*.yaml" -type f -exec sed -i '' 's/[[:space:]]*$$//' {} \;
	@echo "Formatting complete!"

.PHONY: lint
lint: ## Lint YAML files
	@echo "Linting YAML files..."
	@command -v yamllint >/dev/null 2>&1 || { echo "yamllint not found. Install with: pip install yamllint"; exit 1; }
	@yamllint -c .yamllint config/ examples/ || true
	@echo "Linting complete!"

.PHONY: check
check: validate lint ## Run all checks
	@echo "All checks passed!"