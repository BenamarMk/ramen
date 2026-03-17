# Dockerfile Build Fix: replication-storage-io-crds Dependency

## Problem

The Docker build fails with:
```
go: github.com/ramendr/replication-storage-io-crds/api@v0.0.0-00010101000000-000000000000 
(replaced by ./replication-storage-io-crds/api): reading replication-storage-io-crds/api/go.mod: 
open /workspace/replication-storage-io-crds/api/go.mod: no such file or directory
```

## Root Cause

The `go.mod` has a `replace` directive:
```go
replace github.com/ramendr/replication-storage-io-crds/api => ./replication-storage-io-crds/api
```

This works locally but fails in Docker because:
1. The `replace` directive points to a local directory
2. That directory isn't in the Docker build context
3. `go mod download` can't find the module

## Solution Options

### Option 1: Publish the Package (RECOMMENDED for Production)

**Steps:**

1. **Tag and push the replication-storage-io-crds package:**
```bash
cd replication-storage-io-crds
git tag api/v0.1.0
git push origin api/v0.1.0
```

2. **Remove the replace directive from go.mod:**
```go
// Remove this line:
// replace github.com/ramendr/replication-storage-io-crds/api => ./replication-storage-io-crds/api
```

3. **Update the require version:**
```go
require (
    github.com/ramendr/replication-storage-io-crds/api v0.1.0  // Use actual version
)
```

4. **Run go mod tidy:**
```bash
go mod tidy
```

5. **Revert Dockerfile changes:**
```dockerfile
# Remove this line:
# COPY replication-storage-io-crds/ replication-storage-io-crds/
```

**Pros:**
- ✅ Clean separation of concerns
- ✅ Package can be versioned independently
- ✅ Other projects can use it
- ✅ No Docker build context issues
- ✅ Follows Go module best practices

**Cons:**
- ❌ Requires publishing to GitHub
- ❌ Need to tag releases
- ❌ Slower iteration during development

---

### Option 2: Keep Replace Directive (For Development)

**Steps:**

1. **Keep the Dockerfile change (copy only api/):**
```dockerfile
COPY replication-storage-io-crds/api/ replication-storage-io-crds/api/
```

2. **Keep the replace directive in go.mod:**
```go
replace github.com/ramendr/replication-storage-io-crds/api => ./replication-storage-io-crds/api
```

**Pros:**
- ✅ Fast iteration during development
- ✅ No need to publish
- ✅ Easy to test changes

**Cons:**
- ❌ Increases Docker image build context
- ❌ Not production-ready
- ❌ Couples the packages

---

### Option 3: Hybrid Approach (RECOMMENDED for Now)

Use replace directive for development, but prepare for production:

**For Development (Current):**
```go
// go.mod
replace github.com/ramendr/replication-storage-io-crds/api => ./replication-storage-io-crds/api
```

```dockerfile
# Dockerfile
COPY replication-storage-io-crds/api/ replication-storage-io-crds/api/
```

**For Production (Future):**
1. Publish the package with a version tag
2. Remove the replace directive
3. Remove the COPY line from Dockerfile
4. Update require to use the published version

---

## Recommended Path Forward

### Phase 1: Development (NOW)
```bash
# 1. Keep current Dockerfile with COPY line
COPY replication-storage-io-crds/api/ replication-storage-io-crds/api/

# 2. Keep replace directive in go.mod
replace github.com/ramendr/replication-storage-io-crds/api => ./replication-storage-io-crds/api

# 3. Build and test
make docker-build IMG=<your-image>
```

### Phase 2: Prepare for Production (NEXT)
```bash
# 1. Create a release branch for replication-storage-io-crds
cd replication-storage-io-crds
git checkout -b release/v0.1.0

# 2. Tag the release
git tag api/v0.1.0
git push origin api/v0.1.0

# 3. Verify the tag is accessible
go list -m github.com/ramendr/replication-storage-io-crds/api@v0.1.0
```

### Phase 3: Production Build (FUTURE)
```bash
# 1. Remove replace directive from go.mod
# 2. Update require to use published version
# 3. Remove COPY line from Dockerfile
# 4. Run go mod tidy
# 5. Build and test
```

---

## Quick Fix for Your Current Build

**Immediate solution to unblock your build:**

1. **Revert Dockerfile to original** (remove the COPY line I added)
2. **Copy only the api directory:**

```dockerfile
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY api/ api/
# Copy only the API types (minimal footprint)
COPY replication-storage-io-crds/api/ replication-storage-io-crds/api/
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
```

This copies only the `api/` subdirectory (go.mod, go.sum, and .go files), not the entire replication-storage-io-crds package.

**Size comparison:**
- Full package: ~500KB (includes config/, examples/, docs/)
- API only: ~50KB (just Go types and module files)

---

## Alternative: Use Multi-Stage Build

If you want to avoid copying the package at all:

```dockerfile
# Stage 1: Vendor dependencies
FROM golang:1.24 as vendor
WORKDIR /workspace
COPY go.mod go.sum ./
COPY api/ api/
COPY replication-storage-io-crds/api/ replication-storage-io-crds/api/
RUN go mod download
RUN go mod vendor

# Stage 2: Build
FROM golang:1.24 as builder
WORKDIR /workspace
COPY --from=vendor /workspace/vendor vendor/
COPY go.mod go.sum ./
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -a -o manager cmd/main.go

# Stage 3: Runtime
FROM registry.access.redhat.com/ubi8/ubi-minimal
WORKDIR /
COPY --from=builder /workspace/manager .
COPY LICENSES/Apache-2.0.txt licenses/Apache-2.0.txt
USER 65532:65532
ENTRYPOINT ["/manager"]
```

---

## Summary

**Your question:** "Don't I need only the types from it?"

**Answer:** Yes, you're absolutely right! You only need the types. Here are your options:

1. **Best for now:** Copy only `replication-storage-io-crds/api/` (not the whole package)
2. **Best for production:** Publish the package and remove the replace directive
3. **Alternative:** Use vendoring to avoid copying anything

The current Dockerfile change I made copies the entire `replication-storage-io-crds/` directory, which is more than needed. We should copy only `replication-storage-io-crds/api/` to minimize the build context.