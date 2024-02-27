# 2. Deny cross-namespace refs by default

* Status: [ **proposed** | rejected | accepted | deprecated ]
* Date: 2023-07-18
* Authors: @squaremo
* Deciders: [list of GitHub handles for those that made the decision]

## Context

Like [Flux](https://fluxcd.io/), the tofu-controller API has a handful
of places where it accepts cross-namespace references.

 - `Terraform.spec.sourceRef` -- refers to the Flux source object with
   the Terraform program
 - `Terraform.spec.dependsOn[]` -- refers to other objects that must
   be ready before this object can be run
 - `.data.resources[]` -- in the config struct used by the branch
   planner

In general in Kubernetes, references to objects in other namespaces
are frowned upon, because

 - they break namespace isolation assurances; and,
 - they encourage the proliferation of permissions.

Both of these effects make a system less secure.

However: removing cross-namespace refs entirely would break some
installations in a way that would be difficult to fix, because Flux
deployments often rely on defining sources away from objects that use
them.

## Decision

Deny cross-namespace references by default, but allow them to be
enabled with a flag.

So that the default value means the right thing, the flag name must be
`enable-cross-namespace-refs`, and the default `false`. To avoid
confusion when people try to use the Flux version of this flag
`--disable-cross-namespace-refs`, it should be supported too, but only
respected if supplied.

## Consequences

The changed default will break deployments that rely on
cross-namespace refs, but they are easily fixed with the flag.

New deployments will be more secure, by default.
