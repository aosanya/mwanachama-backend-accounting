# Hub import drift

`mwanachama-backend-accounting` owns the accounting module. The hub holds an in-tree copy (`projects/hub-steward/code/wakala-api/modules/accounting`) that drifted: it carries code this repository lacked. That code moved here, with its import path rewritten to `github.com/aosanya/mwanachama-backend-accounting`, so this repository is now the union of both sides. The hub's copy is untouched. Once this repository is tagged, the hub drops its copy and imports the tag.

The direction is one way: hub to this repository, never the other. Nothing here is copied into the hub.

## What moved unchanged

Each file was taken from the hub with only its imports rewritten.

| File | What it holds |
|---|---|
| `models/bill.go`, `bill_store.go`, `bill_store_test.go` | Bills: an invoice raised against an income account and a receivable account, read back with what is settled, what is outstanding and whether it is open, overdue, settled or reversed. A bill is an entry with a due date, so it needs no role of its own. Payer actor and payer record row dimensions. |
| `models/payment.go`, `payment_store.go`, `payment_store_test.go` | Payments: a payment against a bill (how it was paid and a reference, a payment dated in the future refused), a payment reversed as a new line, and the payments of milestone certificates, listed together. |
| `models/procurement.go`, `procurement_store.go`, `procurement_test.go` | Tenders drawn against a budget line and the supplier quotes offered for them. Two roles, `tender` and `quote`. |
| `models/update.go`, `update_store.go`, `update_test.go` | Correcting a bill, a tender or a quote, refused with `ErrLocked` once the record can no longer change. |
| `models/report.go`, `report_store.go`, `report_test.go` | The report fold: planned, spent and difference per period and per department, with the closing balance of each account type. |
| `models/upward_report.go`, `upward_report_store.go`, `upward_report_test.go` | The upward report: the report as it stood when it was sent, kept as a record. One role, `upward_report`. |
| `budget_line_test.go` | Budget lines read back with their name, department, planned and spent. |

## What was merged by hand into shared files

| File | What the hub added, now here | What stayed from this repository |
|---|---|---|
| `models/ledger.go` | `Budget.Name`, `Budget.Department`, `Budget.Create`; `BudgetLine`; `DimensionTotals.Without`; `DimensionEntry`, `DimensionEntries`. | The cost shares and the milestone switch on a budget. |
| `ledger.go` | Aliases for `BudgetLine`, `DimensionEntry`, `DimensionEntries`; `BudgetLineFilter`. | |
| `errors.go` | `ErrLocked`; `BudgetLineExistsError`; the `ErrBudgetLineInUse` text now names a tender drawing on the line. | The figure and day errors. |
| `budget_store.go` | `SetBudget` refuses a second line for the same account and period when `create` is true; `GetBudgetLine`, `ListBudgetLines`; the version check; `DeleteBudgetLine`. | The delete also refuses while a tender draws on the line and answers the cost shares and the milestone switch. |
| `dimension_totals.go` | `Without`, the total of the entries that carry no value for the key; `DimensionEntries`. | The cost shares in the totals. |
| `ledger_repository.go` | The procurement, report, upward report, bill and payment repositories; `ListBudgetLines`, `GetBudgetLine`, `DimensionEntries`, `DeleteBudgetLine`. | The day repository, `AppendFigure`, `ListFigureHistory`. |
| `ledger_store.go` | Carriers for `tender`, `quote` and `upward_report`. | Carriers for `figure`, `day_allowance`, `day_entry`. |
| `accounting.blueprint.json` | `name` and `department` on the budget; the roles `tender`, `quote`, `upward_report`. | The roles `figure`, `day_allowance`, `day_entry`. |
| `accounting.operations.json` | Twenty operations: `list_budget_lines`, `get_budget_line`, `list_reports`, `get_report`, `send_upward_report`, `list_upward_reports`, `create_tender`, `list_tenders`, `get_tender`, `create_quote`, `update_tender`, `update_quote`, `list_quotes`, `create_bill`, `list_bills`, `get_bill`, `update_bill`, `mark_bill_paid`, `list_payments`, `dimension_entries`; `delete_budget_line` and `reverse_payment`; the errors `ErrLocked`, `ErrStale` and `ErrBudgetLineExists`; the longer descriptions of `set_budget` and `dimension_totals`. | The figure and day operations. |
| `mwanachama.accounting.json` | The objects `tender`, `quote`, `upward_report`. | The objects `figure_entry`, `day_allowance`, `day_entry`. |
| `routes/routes.go` | The sentinels `ErrLocked`, `ErrStale` and `ErrBudgetLineExists`; the exported `Sentinels()`. | The figure and day sentinels. |
| `dimension_totals_test.go`, `dimension_shares_test.go` | The hub's tests of `Without`, of the entries of one value and of reading a line back with `GetBudgetLine`, added beside this repository's tests. | Every existing test. |

## Roles are optional

The hub's store requires every role. This repository already treated `figure`, `day_allowance` and `day_entry` as roles a domain may leave out, and the new roles follow the same rule: a domain spec that does not fill `tender`, `quote` or `upward_report` still builds a ledger, and the operations on those roles answer `ErrInvalid` ("this domain keeps no tenders"). `spec/examples/school.accounting.json` fills none of them and is covered by `optional_roles_test.go`. The shipped `mwanachama.accounting.json` fills all of them, so the hub sees no difference.

## Pinned counts changed

Both sides now live here, so the pins moved. No test was weakened.

| Test | Was | Now |
|---|---|---|
| `blueprint_test.go`, roles the blueprint declares | 8 | 11 (`quote`, `tender`, `upward_report` added) |
| `routes/routes_test.go`, addresses the routes answer | 29 | 50 (the twenty operations above and `reverse_payment`) |
| `routes/routes_cover_test.go`, shaped routes | 29 | 50 |

## Tests added here

The hub tested the new operations through its tool surface, which this repository does not carry. The same behaviour is covered over HTTP in `routes/billing_routes_test.go` and `routes/reporting_routes_test.go`, and the optional roles and the tender guard on deleting a budget in `optional_roles_test.go`.

## Shape order moved

`routes.Shape()` is ordered by operation name, so the twenty-one new operations shift the index of every route after them. A consumer that mounts `Shape()` by index must read the table again after taking the tag.

## Resolved: the version check on a budget line

The hub's `set_budget` takes a `version` (the `set_at` the caller read) and answers `ErrStale` when the line changed since; its delete takes the same. The check is `versioncheck`, a package that exists only in the hub's in-tree shared copy and not in `mwanachama-backend-shared`. It now lives here, in `internal/versioncheck`, a copy of the hub's package with the same behaviour: an empty version is no check, a version equal to the stored `set_at` to the second passes, and anything else is refused naming the first changed field (`name`, `department` or `amount` on a set, `record` on a delete).

| Piece | Where |
|---|---|
| `Budget.Version` | `models/ledger.go`, not stored (`spec:"-"`) |
| `staleBudget`, `staleBudgetLine`, the check inside `SetBudget` and `DeleteBudgetLine` | `budget_store.go` |
| The `ErrStale` sentinel, mapped by the type `StaleError`, status 409 | `routes/routes.go`, `accounting.operations.json` |
| The `version` argument of `set_budget` (in the body) and of `delete_budget_line` (a query argument) | `accounting.operations.json` |

A set of a line that does not exist yet has nothing to be stale against and is accepted with any version.

When `versioncheck` lands in `mwanachama-backend-shared`, `internal/versioncheck` is deleted and the three imports switch to the shared package. `ErrStale` keeps mapping by type, so the wire does not change.

## Resolved: one delete, the budget line

Both sides declared `DELETE /budgets/{budgetID}` under the action `accounting.budget.delete` and answered differently. This repository now takes the hub's shape, so the two are the same:

| | Now |
|---|---|
| Operation and call | `delete_budget_line`, `DeleteBudgetLine(ctx, id, version)` |
| Answer | The `BudgetLine` that was removed (`name`, `department`, `planned`, `spent`, and also `follow_milestones` and `cost_shares`, which the hub's line did not carry) |
| Version check | Refused with `ErrStale` when the line changed since |
| Refused while a tender draws on it | Yes, `ErrBudgetLineInUse` |

The repository method `DeleteBudget` and the operation `withdraw_budget` are gone. A consumer that called `DeleteBudget` calls `DeleteBudgetLine(ctx, id, "")` and reads a `BudgetLine`.

## Payment reversal (hub MUN1534)

The hub gained `reverse_payment` after the base this page was written against: `POST /payments/{paymentID}/reversal`, action `accounting.payment.reverse`, status 201, the tool `accounting_payment_reverse`, with `reversed_by` supplied by the mount's caller. `ReversePayment` posts a line that undoes a payment of a bill and names it in `reverses_entry_id`; it is refused once done (`ErrAlreadyReversed`, 409), for anything that is not a payment of a bill (`ErrInvalid`, 400) and for an unknown id (`ErrNotFound`, 404). The routes count is now 50. The hub's permission lists `accounting.payment.reverse` under `accounting.manage` only.

## The error-name map is exported

`routes.Sentinels()` returns a copy of the map from the names the operations file declares to the errors, `ErrStale` and `ErrBudgetLineExists` included. The hub's tool filter builds its tools with it and no longer keeps its own copy.

## For the hub's import step

Not moved, because it does not belong to the module:

| Hub file | What it is |
|---|---|
| `routes/tools.go`, `routes/procurement_tools.go` | The filter that picks which operations become agent tools (budget, bill, payment, report, upward report, procurement, and a few reads), built on `dispatch.Tools`. This repository has no tool filter. |
| `mcp/resolve.go` | Registers those tools on an MCP server per agency instance, using the `modelcontextprotocol/go-sdk`. |
| `routes/*_tools_test.go`, `routes/budget_create_test.go` | Tests of the hub's tool filter. They call `BuildTools`. The version and delete tool tests moved here as `routes/budget_version_test.go` and `routes/budget_delete_route_test.go`, built on `dispatch.Tools`. |
| `routes/permissions_catalog_test.go` and the permission catalogue it reads in the hub's agency module | The `accounting.view` and `accounting.manage` permissions and the actions each holds. The new actions (`accounting.bill.*`, `accounting.payment.*`, `accounting.tender.*`, `accounting.quote.*`, `accounting.report.*`, `accounting.upward_report.*`, `accounting.budget.list`, `accounting.budget.read`, `accounting.dimension.entries`) must stay listed there. |
| `routes/dimension_totals_test.go` (tool section), `routes/routes_cover_test.go` | Hub test fixtures built on the tools. |
| The hub's pinned tool and role counts | They count the tool surface and the hub's roster, and move when the hub swaps its copy for the import. |

The hub's `go.mod` gains this repository at the tag, and its `wakalaserver/modules/accounting` imports become `github.com/aosanya/mwanachama-backend-accounting`. Its `BuildTools` takes the error map from `routes.Sentinels()`. Its day and figure operations, and the figure and day roles, arrive with the import and need their own tools and permissions if the hub wants them served.

## Order

1. Tag this repository (after the owner's yes to tag and push).
2. When `versioncheck` lands in `mwanachama-backend-shared`, drop `internal/versioncheck` here.
3. The hub swaps its in-tree copy for the tag, keeps the wiring listed above and updates its pins.
