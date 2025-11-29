# 0. We will use MADR for Architectural Decision Records

* Status: proposed
* Deciders: @bartsmykla
* Date: 2025-11-29

## Context and Problem Statement

We want to record architectural decisions made in this project.
Which format and structure should we use for that?

## Decision Drivers

* We need a lightweight format that is easy to learn and use.
* The format should be structured to ensure all important aspects of a decision are captured.
* The resulting documents should be easy to read and understand.
* We want to follow a standard that is recognized and used by the community.

## Considered Options

* [MADR](https://adr.github.io/madr/)
* [Michael Nygard's template](http://thinkrelevance.com/blog/2011/11/15/documenting-architecture-decisions)
* [Other templates listed at adr.github.io](httpss://adr.github.io/)

## Decision Outcome

Chosen option: "MADR", because it provides a good balance between structure and simplicity. It is lightweight and has a clear focus on the decision itself, the context, and the consequences. The template is well-defined and easy to follow.

### Positive Consequences

* We have a clear and structured way to document architectural decisions.
* The decisions are easy to find, read, and understand.
* The format is well-known and supported by tools.

### Negative Consequences

* None identified.

## Links

* [MADR specification](https://adr.github.io/madr/)