# 1. Use ADRs to record decisions

* Status: pending
* Date: 2023-06-20
* Authors: @yitsushi
* Deciders: @yitsushi @chanwit @squaremo @yiannistri

## Context

Decisions that affect the development of Terraform Controller that are not
captured via a proposal need to be captured in some way. We need a method that
is lightweight and easy to discover the decision that have been made. The record
of decisions will help future contributors to the project to understand why
something has been implemented or is done a certain way.

## Decision

The project will use [Architectural Decision Records
(ADR)](https://adr.github.io/) to record decisions that are made outside of a
proposal.

A [template](./0000-template.md) has been created based on prior work:

* https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
* https://adr.github.io/madr/

## Consequences

When decisions are made that affect the entire project then a new ADR needs to
be created. Likewise, if a decision has been superseded then we need to capture
this as a new ADR and mark the previous ADR as superseded. Maintainers and
contributors will need to decide when an ADR is to be created.
