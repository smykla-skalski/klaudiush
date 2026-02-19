# 0. We will use MADR for architectural decision records

* Status: proposed
* Deciders: @bartsmykla
* Date: 2025-11-29

## Context and problem statement

We want to record architectural decisions made in this project.
Which format and structure should we use for that?

## Decision drivers

* The format should be lightweight and easy to learn.
* It should capture all the important parts of a decision without extra overhead.
* The resulting documents should be readable at a glance.
* We prefer a format the community already recognizes and uses.

## Considered options

* [MADR](https://adr.github.io/madr/)
* [Michael Nygard's template](http://thinkrelevance.com/blog/2011/11/15/documenting-architecture-decisions)
* [Other templates listed at adr.github.io](https://adr.github.io/)

## Decision outcome

Chosen option: "MADR", because it is structured enough to be useful without getting in the way. It focuses on the decision, the context, and the consequences. The template is straightforward to follow.

### Positive consequences

* Architectural decisions get documented in a consistent place with a consistent shape.
* Decisions are easy to find and read.
* Tooling already supports the format.

### Negative consequences

* None identified.

## Links

* [MADR specification](https://adr.github.io/madr/)
