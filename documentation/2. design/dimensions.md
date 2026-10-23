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

A reversal does not inherit its original's dimensions: it carries what the
caller posts. A caller wanting the narrowed balance to net to zero after a
reversal posts the reversal with the same dimensions.

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

## What this does not do

- **It filters in Go, not in SQL.** Both reads fold the account's whole ledger
  and narrow it, the same cost as `Balance`. A JSON-path index (and the dialect
  split between `jsonb` and SQLite's `json_extract`) is the next step if an
  account's ledger grows past what one fold can carry.
- **No several-key or range narrowing**, no "group by key" read that answers
  every value's total at once, and no narrowing on the budget or overdue reads.
- **No declared vocabulary of keys or values**, and no requirement that an
  entry carry any key — which keys a deployment uses is the deployment's.
