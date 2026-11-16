# Hub copy diff

`mwanachama-backend-accounting` is the custodian of the accounting module. The hub (`projects/hub-steward/code/wakala-api/modules/accounting`) holds a copy made on 2026-11-04 and changed it afterwards. Every change made in that copy exists here, so the two differ by the import path only (hub issue W58, the accounting part of ISS-105).

## Command

```
scripts/hub-copy-diff.sh [path-to-hub-modules-accounting]
```

The default path is `../projects/hub-steward/code/wakala-api/modules/accounting`, relative to this repository's parent folder. The module path and the module's name are read from `go.mod`, so the script carries nothing specific to accounting. The exit code is 1 when the trees differ, 0 when they do not. The script and `scripts/hubdiff` are the ones of `mwanachama-backend-git` (G78) and `mwanachama-backend-shared` (S65).

## What it does

1. Copies the `*.go` and `*.json` files of this repository and of the hub folder into two temporary folders, leaving out `cmd/`, `documentation/`, `scripts/`, `spec/` and `.git/`. The `spec/examples` school domain is a fixture that only this repository has.
2. Rewrites the hub's imports: `wakalaserver/modules/shared/` becomes `github.com/aosanya/mwanachama-backend-shared/` and `wakalaserver/modules/accounting` becomes `github.com/aosanya/mwanachama-backend-accounting`.
3. Runs every Go file on both sides through `scripts/hubdiff`, which parses it, drops all comments and prints it with `gofmt`, so comments and layout do not show as differences.
4. Runs `diff -r backend hub`.

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
