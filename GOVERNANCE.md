# Governance

Evidra is currently maintained with a lean maintainer model so the project can
move quickly while it is still tightening its core product surface.

## Project Scope

The supported core of Evidra is:

- CLI workflows
- MCP workflows
- append-only evidence chain integrity
- behavioral signals, scoring, and explanations

Self-hosted ingestion and browsing remain part of the project, but hosted
analytics are explicitly experimental until they share the same engine end to
end.

## Maintainers

The project currently operates with a single active maintainer model.

- `@vitas` is the current maintainer and release owner.

As external usage grows, Evidra can add maintainers based on sustained review,
design, and release participation.

## Decision Making

- Significant user-visible changes should land through pull requests.
- Architectural decisions should be documented in the repository docs before
  broad behavior changes ship.
- The maintainer is responsible for final merge and release decisions while the
  project is in its current single-maintainer stage.

## Contribution Process

- All changes should go through normal GitHub review.
- Contributors must follow the guidance in [CONTRIBUTING.md](CONTRIBUTING.md).
- Commits merged into the project must include a Developer Certificate of
  Origin sign-off.

## Maintainer Growth

New maintainers may be added when contributors show sustained involvement
across code review, design discussion, release hygiene, and issue response.

The project will add more explicit maintainer rotation and voting rules when it
has a broader active maintainer set.
