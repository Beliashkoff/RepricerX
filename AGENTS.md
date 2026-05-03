# AGENTS.md

## Project type

This is a SaaS application. Prioritize correctness, security, maintainability, and production safety.

## General workflow

- Before large changes, inspect the relevant code paths.
- Prefer small, focused changes.
- Do not rewrite unrelated code.
- Do not introduce new production dependencies without explicit approval.
- Update tests when behavior changes.
- Run the smallest relevant validation command after changes.
- Report changed files and validation results.

## Safety rules

- Never print, request, or expose secrets.
- Do not modify `.env` files unless explicitly instructed.
- Do not weaken authentication, authorization, validation, logging, or rate limits.
- For auth, billing, webhooks, tenant isolation, or migrations, ask for review by a specialized subagent.

## Backend expectations

- Keep business logic in the existing service/usecase layer.
- Keep handlers/controllers thin.
- Validate inputs at boundaries.
- Preserve existing error-handling style.

## Frontend expectations

- Reuse existing components and design patterns.
- Keep state management consistent with the current project.
- Handle loading, empty, error, and success states.

## Database expectations

- Migrations must be backward-compatible when possible.
- Add indexes for new high-volume lookup patterns.
- Preserve tenant isolation.
- Avoid destructive schema changes without explicit approval.

## Done means

A task is done only when:
- The requested behavior is implemented.
- Relevant tests/checks were run or clearly explained if unavailable.
- The diff is focused.
- Remaining risks are reported.
