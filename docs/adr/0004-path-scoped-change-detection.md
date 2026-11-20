# 4. Path-scoped Change Detection

* Status: [ **proposed** | rejected | accepted | deprecated ]
* Date: 2026-09-15
* Authors: @mloiseleur
* Deciders: TBD

## Context

The Tofu Controller reconciles a `Terraform` object on every commit to its source, whether or not
the commit touched anything that object depends on. Every trigger and gate keys on the source
*revision*, never on content — `shouldReconcile`, `requestsForRevisionChangeOf`, the retry reset,
and the pending-plan invalidation cases in `Reconcile`.

A `GitRepository` revision is `<branch>@sha1:<commit>` and changes on every commit. A monorepo with
N `Terraform` objects on one `GitRepository` therefore costs N runner pods and N `terraform plan`
runs per commit, regardless of how many of those paths the commit touched. On a busy monorepo this
dominates controller load and delays the reconciles that do matter.

This was raised as [#9](https://github.com/flux-iac/tofu-controller/issues/9) in 2022. The fix that
landed was narrower than the title suggests: a `shouldDetectDrift` branch restores drift detection
*after* such a plan; it does not prevent the plan.
[#899](https://github.com/flux-iac/tofu-controller/issues/899) asks for the same capability scoped
to the branch planner and is open.

Flux's own lever — [`.sourceignore` / `spec.ignore` on the `GitRepository`][excluding-files], which
leaves `.status.artifact.digest` unchanged when only excluded files move — is per-source, not
per-path. It cannot distinguish two `Terraform` objects on different subdirectories of one
repository.

[excluding-files]: https://fluxcd.io/flux/components/source/gitrepositories/#excluding-files

## Proposal

An opt-in `spec.changeDetection` stanza declares what the object depends on. The controller hashes
that content and skips reconciliation when a new source revision leaves the hash unchanged.

```yaml
spec:
  path: ./envs/prod/network
  interval: 1h
  changeDetection:
    enabled: true
    extraPaths:
      - ./modules/network
      - ./modules/tags
```

The decisions below refer to these types and to the digest format.

<details>
<summary><b>API</b> — <code>spec.changeDetection</code> and the two new status fields</summary>

```go
// ChangeDetection restricts reconciliation to source revisions that actually
// change the inputs this Terraform object depends on.
// +optional
ChangeDetection *ChangeDetection `json:"changeDetection,omitempty"`

type ChangeDetection struct {
	// Enabled turns on content-based change detection. When false (default),
	// every new source revision triggers a reconciliation.
	Enabled bool `json:"enabled"`

	// ExtraPaths are additional directories or files, relative to the source
	// root, whose content participates in the change hash alongside Spec.Path.
	// Use this for shared modules referenced from outside Spec.Path, e.g.
	// `module { source = "../../modules/network" }`.
	// To exclude files, use .sourceignore or GitRepository spec.ignore, which
	// keep them out of the source artifact entirely.
	// +optional
	ExtraPaths []string `json:"extraPaths,omitempty"`

	// IncludeVarsFrom hashes the keys selected by each Spec.VarsFrom reference,
	// so a change to variable inputs triggers a reconciliation even when the
	// source content is unchanged.
	// Not omitempty: with a true default, omitempty would drop an explicit
	// false on serialization and the API server would re-default it to true.
	// +kubebuilder:default=true
	// +optional
	IncludeVarsFrom bool `json:"includeVarsFrom"`
}
```

Status:

```go
// LastHandledSourceDigest is the content digest of the Terraform sources and
// variable inputs at the last completed reconciliation. Used by change
// detection to decide whether a new source revision is relevant.
// +optional
LastHandledSourceDigest string `json:"lastHandledSourceDigest,omitempty"`

// LastAppliedSourceDigest is the content digest at the last successful apply.
// Used by change detection to decide whether drift detection should run.
// +optional
LastAppliedSourceDigest string `json:"lastAppliedSourceDigest,omitempty"`
```

No new timestamp field. `Status.LastSuccessfulReconcileAt` already exists and already drives the
interval gate in `shouldReconcile`.

</details>

<details>
<summary><b>Digest format</b> — versioned, stored in <code>status</code>, therefore a compatibility contract</summary>

`sourceDigest(tarball, spec) -> string`, sha256 over a canonical byte stream.

Include set, as paths relative to the source root: `spec.path` (empty or `.` means the whole tree)
and each entry of `changeDetection.extraPaths`, both recursive. For each entry, sorted bytewise by
path, write:

```
<relative path> \0 <octal file mode> \0 <sha256 of content> \n
```

Then, when `includeVarsFrom` is true, for each `spec.varsFrom` ref in spec order — over the keys
its `varsKeys` selects, or every key when `varsKeys` is unset. References are always same-namespace,
so no namespace appears in the stream:

```
<kind> \0 <name> \0 <optional> \n
  <key> \0 <sha256 of value> \n     # per selected key, sorted
```

Hashing only the selected keys matters: hashing every key would make an unrelated key in a shared
Secret force exactly the reconcile this feature exists to avoid.

Hash that stream. The result is prefixed with a scheme version (`v1:`) so the format can change
without a stampede — a version bump makes every digest differ, which forces one reconcile per
object and then settles.

Notes:

* Content is hashed per file before entering the stream, so raw Secret bytes never accumulate in a
  single buffer and never reach logs or status.
* File mode is included so a chmod on a script consumed by `local-exec` is not silently dropped.
* A missing `extraPaths` entry is a hard error, not an empty hash. Silently hashing nothing would
  skip applies forever.
* Symlinks are hashed by target path, not followed. Following them can escape the source root.
* Only the single composite digest is written to `status`, never per-value hashes: `status` is
  readable by anyone with `get` on the CR, and sha256 of a low-entropy variable value is guessable
  offline.

</details>

## Decision

1. **Content hash, scoped per object, not artifact digest.**
   * Scoping is per `Terraform` object rather than per source, so objects on disjoint paths of one
     repository no longer wake each other — which is exactly what `.sourceignore` cannot express.

2. **Shared modules are declared, not inferred.**
   * `spec.changeDetection.extraPaths` lists directories outside `spec.path` whose content
     participates in the hash, for `module { source = "../modules/x" }` references. Entries are
     resolved relative to the source root and may not escape it.
   * Auto-discovery by parsing HCL is rejected for the initial implementation. `source` can be a
     variable, and `file()`, `templatefile()`, and `.tfvars` references escape the module graph. A
     missed dependency produces a silently skipped apply, which is this feature's worst failure
     mode.
   * Auto-discovery may be added later as a union with `extraPaths`, never as a replacement.

3. **No per-object exclusion field. `.sourceignore` is the exclusion mechanism.**
   * Files excluded by `.sourceignore` or `GitRepository.spec.ignore` are absent from the artifact
     tarball, so they cannot enter the hash. Path scoping covers everything outside `spec.path` and
     `extraPaths`, leaving only noise *inside* a Terraform directory, which `.sourceignore` handles.
   * Accepted limitation: `.sourceignore` is source-wide. It cannot express "exclude this file from
     the Terraform hash but keep it in the artifact for other consumers of the same
     `GitRepository`".

4. **Variable inputs participate in the hash.**
   * `spec.varsFrom` Secrets and ConfigMaps are hashed alongside file content, so a variable change
     is never mistaken for "nothing changed".
   * This is a new controller-side read — `varsFrom` is resolved runner-side today. Existing RBAC
     already permits it; no chart change.

5. **Opt-in, additive on `v1alpha2`, no new API version.**
   * `spec.changeDetection.enabled` defaults to `false`. An object without the stanza behaves
     exactly as today.
   * The field and the two status fields are optional and additive, which is backward-compatible
     within an API version. `v1alpha2` is the storage version.

6. **The gate lives in `shouldReconcile`, and falls through rather than returning early.**
   * `shouldReconcile` is already the throttle and already owns the interval logic. Its
     source-revision rule compares the current digest against `LastHandledSourceDigest`; when they
     match it declines to force a reconcile and the remaining rules still run — including the
     existing interval gate on `Status.LastSuccessfulReconcileAt`. A bare early return would freeze
     `LastAttemptedRevision` and stop drift detection.
   * The digest is therefore computed before `LookupOrCreateRunner`, earlier than today's download
     inside `setupTerraform`, since runner pod spin-up is the cost being avoided. A cache keyed on
     `meta.Artifact.Digest` keeps concurrent reconciles of one revision to a single download; it is
     advisory, so a miss costs a download and never a wrong answer.

7. **Revision status fields keep their literal meaning; drift detection gates on digests.**
   * `LastAppliedRevision` and `LastPlannedRevision` never advance to a revision at which no apply
     or plan ran. `LastAttemptedRevision` *does* advance on a skip — the object attempted this
     revision and concluded there was no work — which also stops `requestsForRevisionChangeOf`
     re-enqueuing on every subsequent watch event.
   * Because `LastAppliedRevision` legitimately lags, `shouldDetectDrift` cannot use its revision
     triple-equality under change detection. It instead compares `lastAppliedSourceDigest` against
     the current digest, in a parallel branch gated on `changeDetection.enabled`.

## Consequences

1. Reconciles caused by unrelated commits are eliminated. With drift detection **disabled**, those
   commits are dropped entirely.

2. With drift detection **enabled**, one runner pod per object per `spec.interval` remains an
   unavoidable floor. Change detection removes only the reconciles above that floor, and the docs must say so.

3. The pending-plan invalidation cases in `Reconcile` compare revisions directly and need the same
   guard, otherwise an unrelated commit still clears a pending plan and forces a re-plan.

4. The controller reads Secret material it previously never touched. `includeVarsFrom: false` is
   the escape hatch for users who consider even a composite digest over Secret material in `status`
   unacceptable.

5. Two configurations silently skip applies: a misconfigured `extraPaths`, and a remote module
   pinned to a mutable ref — if the ref is repointed, `terraform init` fetches new code but the
   digest does not move. Without change detection an unrelated commit would eventually catch the
   latter by accident. Documentation must require immutable refs when the feature is enabled.

6. Users relying on `.sourceignore` for exclusion must understand it affects every consumer of that
   `GitRepository`, including Flux `Kustomization` objects. Needs a documented warning.

7. The hash helper is reusable by the branch planner to answer "does this PR touch my Terraform?",
   which is what [#899](https://github.com/flux-iac/tofu-controller/issues/899) asks for. Not in
   scope here.
