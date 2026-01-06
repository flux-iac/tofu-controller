# Copilot Instructions for Tofu Controller

## Overview
Tofu Controller is a GitOps controller for Flux that reconciles OpenTofu/Terraform resources in Kubernetes. It uses a **Controller/Runner architecture** where the Controller orchestrates operations and communicates with Runner pods via gRPC to execute Terraform commands.

## Architecture & Components

### Core Architecture Pattern
- **Controller** (`controllers/tf_controller*.go`): Main reconciler that manages Terraform CRDs and orchestrates workflows
- **Runner** (`runner/`): gRPC server pods that execute actual Terraform operations (init, plan, apply, destroy)
- **API** (`api/v1alpha2/`): Kubernetes Custom Resource Definitions for Terraform resources
- **Communication**: Controller ↔ Runner via gRPC over port 30000 with mTLS authentication

### Key Directories
- `controllers/`: Main reconciler logic split by concern (plan, apply, backend, drift detection, etc.)
- `runner/`: gRPC server implementation for Terraform execution
- `api/v1alpha2/`: Primary API version with Terraform CRD types
- `cmd/manager/`: Controller manager entry point
- `cmd/runner/`: Runner pod entry point  
- `cmd/tfctl/`: CLI tool for interacting with Terraform resources
- `cmd/branch-planner/`: Branch-based planning component

## Development Workflows

### Building & Testing
```bash
# Run unit tests with proper test environment
make test

# Run specific test by number (e.g. test case 250)
make TARGET=250 target-test

# Build all containers
make docker-build

# Local development with KinD + Tilt
./tools/reboot.sh  # Setup KinD cluster with Flux
export GITHUB_TOKEN=<token>
tilt up  # Auto-rebuilding dev environment
```

### Test Organization
- Tests follow pattern `tc######_description_test.go` with 6-digit numbers
- Integration tests use Ginkgo/Gomega BDD style (`Given`, `By`, `It`, etc.)
- Test environment variables: `INSECURE_LOCAL_RUNNER=1`, `DISABLE_K8S_LOGS=1`, etc.

## Project-Specific Patterns

### Controller Reconciler Structure
Controllers are split by functional concern rather than monolithic:
- `tf_controller.go`: Main reconciler setup and coordination
- `tf_controller_plan.go`: Planning logic (`shouldPlan()`, `plan()`)
- `tf_controller_apply.go`: Apply operations
- `tf_controller_drift_detect.go`: Drift detection workflows  
- `tf_controller_runner.go`: Runner pod lifecycle management
- `tf_controller_backend.go`: Backend configuration handling

### gRPC Communication Pattern
```go
// Standard pattern for Controller → Runner communication
runnerClient, connClose, err := r.LookupOrCreateRunner(ctx, terraform, revision)
defer connClose()
planReply, err := runnerClient.Plan(ctx, &runner.PlanRequest{...})
```

### Runner Operations
Key gRPC methods in `runner/runner.proto`:
- `NewTerraform`: Initialize Terraform session
- `Plan`: Execute terraform plan
- `Apply`: Execute terraform apply  
- `LoadTFPlan`/`SaveTFPlan`: Persistent plan storage
- `UploadAndExtract`: Source code deployment

### Terraform Resource Lifecycle States
- Plan → Apply → Reconcile cycles
- Drift detection with configurable intervals
- Approval workflows (manual vs auto-approve)
- Backend state management (local, remote, or disabled)

## Key Conventions

### File Mapping Pattern
- Use `RunnerFileMappingLocation*` constants for file placement
- Support both `home` and `workspace` locations
- File permissions: directories `0700`, files `0600`

### Error Handling
- gRPC status codes for Runner errors
- Kubernetes Events for user-visible state changes  
- Detailed logging with trace levels for debugging

### Configuration
- Environment variables for test modes and feature flags
- Helm chart values in `charts/tofu-controller/values.yaml`
- mTLS certificates managed by cert-rotator

## Integration Points
- **Flux Source Controller**: GitRepository, Bucket, OCIRepository sources
- **Kubernetes Secrets**: Variable injection, backend config, TLS certificates
- **Pod Specifications**: Runner pods with custom resource limits, security contexts
- **GRPC**: All Terraform execution via protobuf-defined service contracts

When modifying this codebase, ensure gRPC changes regenerate proto files with `make gen-grpc`, maintain the Controller/Runner separation, and follow the established test naming conventions.

# Development Partnership

We build production code together. I handle implementation details while you guide architecture and catch complexity early.

## Core Workflow: Research → Plan → Implement → Validate

**Start every feature with:** "Let me research the codebase and create a plan before implementing."

1. **Research** - Understand existing patterns and architecture
2. **Plan** - Propose approach and verify with you
3. **Implement** - Build with tests and error handling
4. **Validate** - ALWAYS run formatters, linters, and tests after implementation

## Code Organization

**Keep functions small and focused:**
- If you need comments to explain sections, split into functions
- Group related functionality into clear packages
- Prefer many small files over few large ones

## Architecture Principles

**This is always a feature branch:**
- Delete old code completely - no deprecation needed
- No versioned names (processV2, handleNew, ClientOld)
- No migration code unless explicitly requested
- No "removed code" comments - just delete it

**Prefer explicit over implicit:**
- Clear function names over clever abstractions
- Obvious data flow over hidden magic
- Direct dependencies over service locators

## Maximize Efficiency

**Parallel operations:** Run multiple searches, reads, and greps in single messages
**Multiple agents:** Split complex tasks - one for tests, one for implementation
**Batch similar work:** Group related file edits together

## Go Development Standards

### Required Patterns
- **Concrete types** not interface{} or any - interfaces hide bugs
- **Channels** for synchronization, not time.Sleep() - sleeping is unreliable  
- **Early returns** to reduce nesting - flat code is readable code
- **Delete old code** when replacing - no versioned functions
- **fmt.Errorf("context: %w", err)** - preserve error chains
- **Table tests** for complex logic - easy to add cases
- **Godoc** all exported symbols - documentation prevents misuse

## Problem Solving

**When stuck:** Stop. The simple solution is usually correct.

**When uncertain:** "Let me ultrathink about this architecture."

**When choosing:** "I see approach A (simple) vs B (flexible). Which do you prefer?"

Your redirects prevent over-engineering. When uncertain about implementation, stop and ask for guidance.

## Testing Strategy

**Match testing approach to code complexity:**
- Complex business logic: Write tests first (TDD)
- Simple CRUD operations: Write code first, then tests
- Hot paths: Add benchmarks after implementation

**Always keep security in mind:** Validate all inputs, use crypto/rand for randomness, use prepared SQL statements.

**Performance rule:** Measure before optimizing. No guessing.

## Progress Tracking

- **TodoWrite** for task management
- **Clear naming** in all code

Focus on maintainable solutions over clever abstractions.
