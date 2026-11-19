# Hub copy diff

`mwanachama-backend-accounting` is the custodian of the accounting module. The hub (`projects/hub-steward/code/wakala-api/modules/accounting`) holds a copy made on 2026-11-04 and changed it afterwards. Up to 2026-11-16 every change made in that copy existed here (hub issue W58, the accounting part of ISS-105). The copy then grew further, and on 2026-11-19 the code it alone held moved here, so this repository is the union of both. [hub-import-drift.md](hub-import-drift.md) lists what moved and what the hub keeps.

## Command

```
scripts/hub-copy-diff.sh [path-to-hub-modules-accounting]
```

The default path is `../projects/hub-steward/code/wakala-api/modules/accounting`, relative to this repository's parent folder. The module path and the module's name are read from `go.mod`, so the script carries nothing specific to accounting. The exit code is 0 when the output holds no line, or only `Only in backend` lines; it is 1 when any line starts with `diff `, `Binary files ` or `Only in hub`. The script and `scripts/hubdiff` are the ones of `mwanachama-backend-git` (G78) and `mwanachama-backend-shared` (S65).

## What it does

1. Copies the `*.go` and `*.json` files of this repository and of the hub folder into two temporary folders, leaving out `cmd/`, `documentation/`, `scripts/`, `spec/` and `.git/`. The `spec/examples` school domain is a fixture that only this repository has.
2. Rewrites the hub's imports: `wakalaserver/modules/shared/` becomes `github.com/aosanya/mwanachama-backend-shared/` and `wakalaserver/modules/accounting` becomes `github.com/aosanya/mwanachama-backend-accounting`.
3. Runs every Go file on both sides through `scripts/hubdiff`, which parses it, drops all comments and prints it with `gofmt`, so comments and layout do not show as differences.
4. Runs `diff -r backend hub`, prints its output, and sets the exit code by the rule above.

## Result of 2026-11-16

Every non-test Go file and every JSON file is identical after the rewrite, and every test file of the hub exists here and is identical. Nothing in the output is `diff` or `Only in hub`. What remains is `Only in backend` lines, all test files written here that the hub copy never carried: `blueprint_test.go`, `budget_test.go`, `dimensions_test.go`, `domain_agnostic_test.go`, `ledger_test.go`, `overdue_test.go`, `provision_test.go`, `reversal_dimensions_test.go`, `settles_test.go`, `testdb_test.go`, `routes/dimensions_test.go` and `routes/routes_test.go`. They test code that is identical in both.

## The hub commits carried over

One commit here per hub commit, each naming the hub hash:

| Hub commit | What it brought |
|---|---|
| 801ffc4c | The overdue read ages from the day a charge was raised (`age_from`, `age_days`). |
| b065ba34 | Budget milestones, payment certificates and contract retention (two new roles, five operations, three refusals). |
| 712cf2d9, e2fd23af, c237f37d, 3bd6fe3d | The cover tests of the ledger rules, the ledger store, budgets and overdue ageing, and the routes. |
| d8e03aef | Posting the same document line again answers the first entry. |

Three changes here have no hub counterpart and follow from the hub code:

- `Overdue` takes a fourth argument, so the callers in `overdue_test.go` and `settles_test.go` pass an empty `age_from`.
- The module now fills five roles, so `blueprint_test.go` expects five, `routes/routes_test.go` expects seventeen routes, and `spec/examples/school.accounting.json` fills the two new roles.
- The same reversal line posted twice under one document id is now a replay of the first reversal, so the double-reversal tests in `ledger_test.go` and `routes/routes_test.go` post the second reversal under a second document id and still expect `ErrAlreadyReversed`.

## Result of 2026-11-19

After the hub-only code moved here, no line of the output starts with `Only in hub` except what is listed under "Hub-only wiring" below, and every `diff` is one of the following.

| Where | Difference | Why it stays |
|---|---|---|
| `accounting.blueprint.json`, `mwanachama.accounting.json` | This repository declares and fills `figure`, `day_allowance` and `day_entry`; the hub does not. | Only here. They arrive in the hub with the import. |
| `accounting.operations.json` | The figure and day operations here, and the donor wording of the two upward report operations. | Only here. |
| `budget_store.go`, `models/ledger.go`, `ledger_repository.go`, `routes/routes.go` | The day and figure repositories only here; the version check imports `internal/versioncheck` here and the shared copy there; the returned line also carries the cost shares and the milestone switch. | The check waits for `versioncheck` in the shared module. See [hub-import-drift.md](hub-import-drift.md). |
| `ledger_store.go`, `procurement_store.go`, `update_store.go`, `upward_report_store.go` | Here the carriers for `tender`, `quote` and `upward_report` are optional and their operations check that the domain fills the role first. | The hub requires every role; this repository lets a domain leave them out. |
| `dimension_totals_test.go`, `dimension_shares_test.go`, `budget_delete_test.go` | The hub's additions were merged in beside the existing tests; the hub's delete tests moved, and the budget version and delete tool tests became `routes/budget_version_test.go` and `routes/budget_delete_route_test.go`. | Same behaviour. |
| `Only in hub`: `mcp/`, `routes/tools.go`, `routes/procurement_tools.go` and the `routes/*_tools_test.go` files | The agent tool filter, the MCP registration and their tests. | Hub-only wiring, listed in the drift page for the hub's import step. |
| `Only in backend`: the day, figure, route, blueprint and billing, reporting and optional-role tests | Written here. | They test code both sides now hold or only this side holds. |

The script keeps printing `Only in hub` for the wiring above until the hub drops its copy.
