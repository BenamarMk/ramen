# Neutral API Location Strategy

## Your Question

> The neutral API lives within Ramen directory. Will that be changed to live outside of Ramen or your suggestion is to have it within Ramen even though it is external. I am confused.

## Current State (What We Built)

### Phase 6: Standalone Package Within Ramen
```
ramen-with-bob/
├── replication-storage-io-crds/     ← Standalone package (within Ramen repo)
│   ├── api/
│   │   └── v1alpha1/
│   │       ├── volumegroupreplication_types.go
│   │       ├── volumegroupreplicationclass_types.go
│   │       └── common_types.go
│   ├── config/crd/                  ← CRD manifests
│   ├── go.mod                       ← Independent module
│   └── README.md
└── internal/controller/
    └── replication/
        ├── neutral_handler.go       ← Uses replication-storage-io-crds
        └── legacy_handler.go        ← Uses csi-addons API
```

**Key Point:** The package is **standalone** (has its own `go.mod`) but **physically located** within the Ramen repository.

## The Confusion: "External" vs "Location"

### Two Different Concepts

1. **API Ownership/Governance** (External)
   - Who defines the API?
   - Who maintains it?
   - Who approves changes?

2. **Code Location** (Physical)
   - Where does the code live?
   - Which repository?
   - Which organization?

### Current Implementation

| Aspect | Status | Notes |
|--------|--------|-------|
| **API Group** | `replication.storage.io` | Neutral, vendor-agnostic |
| **Governance** | Community/Standard | Follows Kubernetes conventions |
| **Code Location** | Within Ramen repo | `replication-storage-io-crds/` |
| **Module** | Standalone | Has own `go.mod` |
| **Versioning** | Independent | Can version separately |

## Three Possible Strategies

### Strategy 1: Keep Within Ramen (Current - INTERIM)

**Location:** `github.com/ramendr/ramen/replication-storage-io-crds`

**Pros:**
- ✅ Faster development (no cross-repo coordination)
- ✅ Easier testing during development
- ✅ Single PR workflow
- ✅ Immediate availability

**Cons:**
- ❌ Appears Ramen-specific (perception issue)
- ❌ Harder for other projects to adopt
- ❌ Not truly "neutral" in appearance
- ❌ Ramen controls the API

**Best For:** 
- Initial development and validation
- Proof of concept
- Getting feedback from community

### Strategy 2: Move to Kubernetes Community (RECOMMENDED LONG-TERM)

**Location:** `github.com/kubernetes-csi/external-replication` or similar

**Pros:**
- ✅ Truly vendor-neutral
- ✅ Community governance
- ✅ Easier adoption by other projects
- ✅ Follows Kubernetes patterns (like external-snapshotter)
- ✅ Can become a standard

**Cons:**
- ❌ Slower development (community process)
- ❌ Requires KEP (Kubernetes Enhancement Proposal)
- ❌ More coordination needed
- ❌ Longer approval cycles

**Best For:**
- Production adoption
- Industry-wide standard
- Long-term sustainability

### Strategy 3: Separate Neutral Organization (ALTERNATIVE)

**Location:** `github.com/storage-replication/api` or similar

**Pros:**
- ✅ Independent governance
- ✅ Vendor-neutral appearance
- ✅ Faster than Kubernetes community
- ✅ Can evolve independently

**Cons:**
- ❌ Need to create new organization
- ❌ Need to establish governance
- ❌ Less visibility than Kubernetes
- ❌ Adoption may be slower

**Best For:**
- If Kubernetes community process is too slow
- If multiple vendors want to collaborate
- If you want more control than Kubernetes offers

## Recommended Path Forward

### Phase 1: Current State (✅ DONE)
**Keep within Ramen for initial development**

```
Location: github.com/ramendr/ramen/replication-storage-io-crds
Status: Standalone package, independent go.mod
Purpose: Develop, test, validate the API design
```

### Phase 2: Community Proposal (NEXT STEP)
**Propose to Kubernetes community**

1. Write KEP (Kubernetes Enhancement Proposal)
2. Present to sig-storage
3. Get feedback and iterate
4. Build consensus

### Phase 3: Migration (FUTURE)
**Move to kubernetes-csi organization**

```
Old: github.com/ramendr/ramen/replication-storage-io-crds
New: github.com/kubernetes-csi/external-replication
```

**Migration Steps:**
1. Create new repo in kubernetes-csi
2. Move code to new repo
3. Update Ramen to import from new location
4. Deprecate old location
5. Redirect users to new location

## Why This Approach?

### 1. Validate First, Standardize Later

**Current (Within Ramen):**
- Rapid iteration
- Easy to change based on feedback
- No bureaucracy
- Can prove the concept works

**Future (Kubernetes Community):**
- Stable API
- Community buy-in
- Industry adoption
- Long-term support

### 2. Precedent: external-snapshotter

The Kubernetes community followed this pattern:

```
1. Initial development in vendor repos
2. Proof of concept and validation
3. Proposal to Kubernetes community
4. Migration to kubernetes-csi/external-snapshotter
5. Now a widely-adopted standard
```

### 3. Practical Benefits

**For Ramen:**
- Can use the API immediately
- No waiting for community approval
- Can iterate quickly

**For Community:**
- See working implementation
- Evaluate real-world usage
- Make informed decisions

## Answer to Your Question

### Is the current location correct?

**YES, for now.** Here's why:

1. **It's a standalone package** - Has its own `go.mod`, can be versioned independently
2. **It's vendor-neutral in design** - API group `replication.storage.io` (not `ramendr.openshift.io`)
3. **It's ready to move** - When the time comes, we can move it to kubernetes-csi
4. **It's practical** - Allows rapid development without bureaucracy

### What should we do next?

**Short-term (Now):**
- ✅ Keep it in Ramen
- ✅ Use it in production
- ✅ Gather feedback
- ✅ Prove it works

**Medium-term (3-6 months):**
- 📝 Write KEP for Kubernetes community
- 📝 Present to sig-storage
- 📝 Get community feedback
- 📝 Iterate on design

**Long-term (6-12 months):**
- 🚀 Move to kubernetes-csi organization
- 🚀 Establish as community standard
- 🚀 Enable industry-wide adoption

## Comparison with Design Document

### Design Document Vision
> "The Neutral API should be vendor-agnostic and follow community standards"

### Current Implementation
✅ **API Design:** Vendor-agnostic (`replication.storage.io`)
✅ **Code Structure:** Standalone, reusable
✅ **Patterns:** Follows Kubernetes conventions
⏳ **Location:** Within Ramen (temporary)
🎯 **Goal:** Move to community (future)

## Conclusion

**The neutral API is "external" in design and intent, but "internal" in current location.**

This is the **correct approach** because:
1. Allows rapid development and validation
2. Proves the concept works in production
3. Makes it easier to propose to Kubernetes community
4. Follows the pattern of successful Kubernetes projects

**Next Steps:**
1. Continue using current location
2. Gather production feedback
3. Prepare KEP for Kubernetes community
4. Plan migration to kubernetes-csi when ready

The location within Ramen is **temporary and strategic**, not permanent. The API is designed to be moved when the time is right.