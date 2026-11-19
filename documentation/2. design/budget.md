# Budgets and variance (W25)

A budget is its own declared object, `budget`, beside `account` and `entry`.
Before it, a ledger held its budget as a second set of `budget` accounts and
the comparison was two balances read by the caller.

## The object

| Field | Meaning |
| --- | --- |
| `account_id` | the account the budget is set against |
| `period_start` | first instant of the period, inclusive, RFC 3339 |
| `period_end` | end of the period, exclusive, RFC 3339, after the start |
| `amount` | the budgeted balance for the period |
| `follow_milestones` | whether the line is followed milestone by milestone, off by default |
| `cost_shares` | neighbourhood name to whole percent; absent means the cost is counted in one |
| `set_at` | when the amount was last set, written by the store |

One budget per `(account_id, period_start, period_end)`, declared as a unique
index. A period is two instants rather than a named key such as `2026-Q1`,
because a named key is a domain's calendar (a school's term, a council's
financial year) and this module names no domain. The same pair is how the
caller asks for the variance, so the period has to be given exactly as it
was set.

## The two operations

Appended to `accounting.operations.json` under names that sort after
`post_entry`, because `Shape()` orders by operation name and
`mwanachama-wakala-api` pairs routes by that index; the seven existing
indices do not move.

- `set_budget` — `PUT /budgets`, action `accounting.budget.set`. Setting the
  same account and period again replaces the amount and keeps the id. The
  account must exist; a period that does not parse or runs backwards is
  `ErrInvalid`.
- `variance` — `GET /accounts/{accountID}/variance?period_start=&period_end=`,
  action `accounting.budget.variance`. Answers `budgeted`, `actual` and
  `difference` (`actual − budgeted`). `actual` folds the same way `Balance`
  does — debits minus credits — over only the entries whose `occurred_at`,
  or `posted_at` where that is absent, falls inside the period. No budget for
  that period is `ErrNotFound`.

The amount is in the account's debit-minus-credit sense, the same as
`Balance`, so an income account's budget is a negative number. Flipping the
sign by account type would make variance and balance disagree about the same
account.

## Deliberately not built

- **Nothing refuses a posting to an account with no budget.** Whether an
  actual may land without a budget to answer is a product decision for the
  consumer, not a ledger rule, and is left open.
- No listing of budgets; a budget is replaced, not listed. A budget can be deleted only while nothing has been spent in its period (see below).

## Deleting a budget line (MUN1472)

`delete_budget_line` sorts before `dimension_entries` and after the create
operations, so a mount that indexes `Shape()` reads the table again after the
tag.

- `delete_budget_line` — `DELETE /budgets/{budgetID}`, action
  `accounting.budget.delete`, call `DeleteBudgetLine(ctx, id, version)`.
  Removes one budget line and answers the line that was removed (`name`,
  `department`, `planned`, `spent`, `follow_milestones`, `cost_shares`). A
  missing id is `ErrNotFound` (404). It is refused with `ErrBudgetLineInUse`
  (409) when any entry of the line's account falls inside its period, by the
  same `occurred_at`-or-`posted_at` rule the variance uses, so history is
  never dropped from under a figure, and while a tender draws on the line.
  Nothing else changes: the entries stay, and the same account and period can
  be set again afterwards.
- `version` is an optional query argument, the `set_at` of the line as it was
  read. Absent means no check; a version that is not the stored `set_at` to
  the second is `ErrStale` (409) naming `record`.

## Version check on a budget line

`set_budget` takes an optional `version` in the body with the same meaning.
When the line already exists and the version is not its stored `set_at`, the
save is refused with `ErrStale` (409) naming the first of `name`,
`department` and `amount` that differs from what is stored, or `name` when
none does. A refused save changes nothing. A line that does not exist yet has
nothing to be stale against. The version is never stored.

The check is `internal/versioncheck`, a copy of the package the hub keeps in
its own shared copy. When `mwanachama-backend-shared` publishes it, the copy
is deleted and the imports switch; `ErrStale` maps by type, so the wire does
not change.

## Shared cost between neighbourhoods (MUN1409)

Mirrored from the hub copy (hub commits c362e1c7 and 30bbeebb). `set_budget`
stays a whole-body write, so a save replaces `follow_milestones` and
`cost_shares` with what it sends; send them back unchanged to keep them.

- `cost_shares` is checked in `SetBudget`: every key names a neighbourhood,
  every percent is a whole number from 1 to 100 and they sum to 100, else
  `ErrInvalid`. Empty means the cost is counted in one.
- `DimensionTotals` for the key `neighbourhood` splits the spending on an
  account whose line shares its cost, inside the line's period, over the line's
  neighbourhoods by their percents, whatever neighbourhood the entry names or
  whether it names one. The remainder of a division goes to the largest share.
  A reversal takes back the same split. Any other key, entry, account or period
  is folded as before.
- `follow_milestones` is stored and returned; the library has no budget line
  read, so a reader sees it in the answer of `set_budget`.
