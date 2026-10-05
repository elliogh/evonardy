# Task tracking and contribution workflow

[GitHub Issues](https://github.com/elliogh/evonardy/issues) holds tasks, bugs,
acceptance criteria, dependencies, and follow-up work.
[Milestones](https://github.com/elliogh/evonardy/milestones) group work by stage.
The [EvoNardy roadmap](https://github.com/users/elliogh/projects/1) shows the work
as a board, open backlog, M4 view, and completed history.

## Choosing and implementing work

1. Create or select an issue, add it to the Project, and assign its milestone and
   labels. Read its acceptance criteria, prerequisite issues, and the relevant
   versioned technical documentation before starting.
2. Move a defined task to **Ready**. When implementation begins, move it to
   **In progress** and use a branch associated with the issue.
3. Implement and run the checks relevant to its public boundary or user flow.
   Record actual commands, results, compatibility limits, and incomplete work in
   the issue or PR. Use separate issues for follow-up tasks.
4. Move the task to **In review** when a PR is ready. Include `Closes #123` only
   when the PR satisfies the entire issue; merge into the repository's default
   branch after review and required checks. Then mark the task **Done**.
5. Close a milestone only after all its required issues and stage acceptance
   criteria are complete. Refine later stages into sub-issues before implementing
   them. Dates and performance claims require evidence.

The repository templates provide the task, bug, and PR formats. Labels describe
type (`bug`, `enhancement`, `research`, `maintenance`) and area (`engine`,
`training`, `ui`, `storage`); the Project Status field describes work state.

## Required CI

The [CI workflow](https://github.com/elliogh/evonardy/actions/workflows/ci.yml)
runs on every PR targeting `main`, every push to `main`, and manual dispatches.
There are no path exclusions: documentation changes must also pass validation.
Both jobs use Ubuntu 24.04, Go 1.25, and Node.js 24:

- **Checks:** `make setup`, `make test`, `make smoke`, `make build`, and
  `make bench`. Benchmarks record timings and allocations without speed or
  playing-strength thresholds; a failing benchmark command fails the job.
- **Chromium E2E:** `make setup`, Chromium installation with system dependencies,
  and `make e2e`. All nine browser scenarios run with one worker and no retries.

The stable **CI result** check succeeds only when both jobs succeed. Failed,
cancelled, or skipped dependencies fail the gate. The active `main-ci` ruleset
requires a PR, an up-to-date branch, and this check from GitHub Actions before
merging into `main`. There are no bypass actors or required reviewer approvals.
Merge commits remain available to preserve branch history.

To investigate failures, open the PR's Checks tab or the linked Actions run and
inspect the first failing step. Download `checks-logs` for command output and
environment/benchmark metadata, or `chromium-diagnostics` for browser logs,
the HTML report, and retained failure traces. Artifacts expire after seven days
and exclude model data and release binaries. To inspect a downloaded report:

```bash
cd web
npx playwright show-report /path/to/downloaded/playwright-report
npx playwright show-trace /path/to/downloaded/test-results/test-name/trace.zip
```

Fix reproducible failures on the PR branch and push the correction. Rerun jobs
only when investigating a transient runner failure; do not hide failures with
retries or weaker assertions. New commits cancel stale PR runs. Pushes to `main`
and manual dispatches run independently. Update a PR from `main` when its required
check is out of date, then wait for the complete pipeline to pass again.
Long training and model/release publication remain outside ordinary CI.

## Documentation and migration

Rules, architecture, API, model/training contracts, and the implementation brief
remain versioned with code. GitHub is authoritative for tasks and status;
`docs/PROGRESS.md` is an archive of checks before migration and is no longer an
active backlog. Historical issues retain those actual results and local commit
references without inventing PRs or original GitHub completion dates.

The tracker was initially migrated without pushing code. The subsequently
authorized publication introduces the original implementation and templates into
`main` through scoped, issue-linked PRs while preserving the original commits.
Creating tasks does not authorize a push, long training, or a release publication.
