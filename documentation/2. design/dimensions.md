# Dimensions on an entry (W26)

An entry carries an optional `dimensions` object: a flat map of named keys to
text values that the posting is analysed by — a department, a cost centre, a
project, a fiscal year. Before this, an entry moved value between two accounts
and nothing else, so any further split had to be encoded in the account: one
account per value per period, multiplying the chart of accounts by every
combination a report wanted. With dimensions, one account holds the postings
and a read narrows by the dimension instead.

The keys and values are the caller's. This module never learns what any of
them mean, and no key is declared or reserved here — a domain that wants a
fixed set of keys states it in its own client or adapter.

## Shape and rules

- Declared on the blueprint's `entry` as `dimensions`, type `json`, immutable
  like every other entry field. Carried in Go as `map[string]string`; absent
  or empty stores SQL NULL and reads back as no dimensions.
- A key is lower-case letters, digits and underscores, starting with a letter,
  at most 63 characters (`DimensionKeyPattern`). A key reaches no SQL — the
  narrowing is done in Go — but the alphabet keeps keys usable as JSON paths
  if the narrowing ever moves into the database.
- A value is non-empty text, at most 200 characters (`MaxDimensionValue`), with
  no control characters.
- At most 16 keys per entry (`MaxDimensions`).
- A posting breaking any rule is refused with `ErrInvalid` (400) and nothing is
  written. These rules live in Go (`dimensions.go`) because the spec cannot
  state a rule over a document's keys.

A reversal keeps its original's dimensions whatever the caller posts, so the
narrowed balance and the totals below net to zero after a reversal (see "A
reversal keeps its original's dimensions").

## The reads

Both appended at the end of `accounting.operations.json`, so a mount that
indexes `Shape()` keeps its existing positions.

- `GET /accounts/{accountID}/entries/by-dimension?key=&value=[&limit=]`
  (`accounting.entry.list_by_dimension`): the account's entries whose
  dimensions carry `key` with exactly `value`, in the full listing's order,
  capped at the default page.
- `GET /accounts/{accountID}/balance/by-dimension?key=&value=`
  (`accounting.account.balance_by_dimension`): debits minus credits over only
  those entries, answered as `{"balance": n}`.

Each narrows by one key and one exact value. An unknown account is
`ErrNotFound`; a malformed key or an empty value is `ErrInvalid`. The balances
of one account under every value of one key sum to its whole balance only when
every entry carries that key.

## A reversal keeps its original's dimensions

A reversal (an entry naming `reverses_entry_id`) is stored with its original's
`dimensions`, whatever the request names (member app decision 426, W58). Before
this a reversal carried none unless its caller repeated them, so
`DimensionBalance` kept counting a reversed amount under the original's values.
Taking the original's set rather than refusing a mismatch keeps the reversal
a mirror of the original on every read that narrows by a dimension.

## Totals by dimension

One call answers every value of one key at once, instead of one balance call
per value.

- `GET /dimension-totals?key=&account_type=&period_start=&period_end=`
  (`accounting.dimension.totals`, operation `dimension_totals`, appended after
  the other operations). Go: `LedgerRepository.DimensionTotals(ctx, key,
  accountType, periodStart, periodEnd)`.
- Answers `{"key", "account_type", "period_start", "period_end", "totals":
  [{"value", "total", "entries"}]}`; the type and period fields are left out
  when not asked for. `totals` is sorted by value and is an empty list, not
  null, when nothing matches.
- `total` is debits minus credits, and `entries` counts the entries behind it.
- Entries that do not carry the key are skipped, so the rows sum to the
  account-wide figure only when every entry carries the key.

**The sides rule.** An entry has two sides: its amount debits one account and
credits another. A side counts when its account is of the asked type. An entry
with one side on a selected account adds or subtracts its amount once; an entry
with both sides on selected accounts adds and subtracts it, and counts once in
`entries`; an entry with no side on a selected account is not counted at all.

**Why no account type nets to zero.** With no `account_type` every account is
selected, so every entry has both sides selected: the debit adds its amount, the
credit takes the same amount away, and each value totals zero. That is correct
double entry and tells a reader nothing, so a caller wanting a figure names a
type: `expense` for what was spent, `income` for what was raised. `entries` is
still the count of entries under each value. A reversal keeps its original's
dimensions and swaps its sides, so under a type it takes back exactly what the
original added.

**The period.** Optional, and both bounds or neither: one bound alone is
`ErrInvalid`. The bounds are RFC 3339 instants, the start inclusive and the end
exclusive, and an entry falls in when its `occurred_at`, or its `posted_at`
where that is absent, lies inside. This is the rule the budget lines use for
`spent`, so a period given here and a budget line's period read the same
entries.

**Errors.** All `ErrInvalid` (400), nothing partial:

- a key that breaks `DimensionKeyPattern`, including an empty one;
- an `account_type` that is not one of asset, liability, equity, income,
  expense;
- a period bound that is not an RFC 3339 instant;
- a period with only one bound;
- a period whose end is not after its start.

**Example.** A municipality posts four expense entries, each carrying a
`neighbourhood` dimension, from the expense accounts "Road repair" and "Park
upkeep" (the amounts are in the smallest unit):

| Entry | Expense account | `neighbourhood` | Amount |
|---|---|---|---|
| 1 | Road repair | Óváros | 120000 |
| 2 | Road repair | Kertváros | 80000 |
| 3 | Park upkeep | Óváros | 30000 |
| 4 | Park upkeep | Malomsor | 45000 |

`GET /dimension-totals?key=neighbourhood&account_type=expense` answers

```json
{"key":"neighbourhood","account_type":"expense","totals":[
  {"value":"Kertváros","total":80000,"entries":1},
  {"value":"Malomsor","total":45000,"entries":1},
  {"value":"Óváros","total":150000,"entries":2}]}
```

Határrét carries no entry, so it has no row. Without `account_type` the same
call answers a total of 0 for each of the three values, with the same entry
counts. Adding `period_start` and `period_end` narrows the same fold to the
entries dated inside the period.

## What this does not do

- **It filters in Go, not in SQL.** The reads fold the whole ledger (an
  account's for the first two, every entry for the totals) and narrow it, the
  same cost as `Balance`. A JSON-path index (and the dialect
  split between `jsonb` and SQLite's `json_extract`) is the next step if an
  account's ledger grows past what one fold can carry.
- **No several-key or range narrowing**: the totals group by one key, and no
  read groups by two keys or narrows by a range of values. No narrowing on the
  budget or overdue reads either.
- **No declared vocabulary of keys or values**, and no requirement that an
  entry carry any key — which keys a deployment uses is the deployment's.
