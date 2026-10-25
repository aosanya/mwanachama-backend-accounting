# Due dates and the overdue read (W28)

An entry carries an optional `due_at`: the instant what it records falls due.
Absent means the entry has no due date and is never overdue. It is set when the
entry is posted and never changed, like every other entry field; it must be an
RFC 3339 instant or the posting is refused with `ErrInvalid`.

Before this, an entry had only `posted_at` and `occurred_at`, so anything that
aged a balance aged it from when a charge was raised. A ledger that charges
everyone on one common date could live with that; one that charges continuously
could not. The smaller of the two options the row named was built: a due date
on the entry, not an invoice object.

## The read

`GET /accounts/{accountID}/overdue?as_of=<instant>` (`accounting.entry.overdue`,
tool from the same operation) answers `{account_id, as_of, items}`. Each item
is the entry plus `days_overdue`, the whole days between `due_at` and `as_of`.
Items are ordered most overdue first. `as_of` absent means now.

An entry is listed when its `due_at` is strictly before `as_of`, and it is
neither a reversal nor reversed: a reversed charge is not owed, and its
reversal mirrors it and would otherwise list twice. A reversal always touches
the same two accounts as its original, so the account's own entries are enough
to find both.

## A payment settles its charge

An entry may name the charge it pays in `settles_entry_id` (member app
decision 425, W58). `Post` refuses one naming no entry in the ledger with
`ErrNotFound`, the same answer an unknown `reverses_entry_id` gets. The field
is optional and immutable like every other on an entry.

The overdue read nets payments against their charges: for each due entry it
sums the amounts of every entry whose `settles_entry_id` names it, leaving out
a payment that was reversed and a reversal itself, and subtracts that from the
charge's amount. Each item carries what is left as `outstanding`, beside the
charge's own `amount`; a charge whose outstanding is zero or less is not
overdue and is left out. Payments are found by their own index (`settles`),
not by the account's ledger, so a payment posted between two other accounts
still settles the charge it names.

Not decided, so not enforced: that a payment touches the charge's accounts,
that it does not exceed what is outstanding, that it is not itself a charge
with a due date, or that a reversal may not name a charge to settle. Each is a
product call.

## What this does not do

- **Ageing buckets** (0–30, 31–60, …) are the caller's: the read gives the
  days, and the buckets differ by domain.
- It reads one account. A read across a whole category would iterate the
  category's accounts, and was not needed for the row.
