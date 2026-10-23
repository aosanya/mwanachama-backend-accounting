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

## What this does not do

- **It does not net payments against charges.** An entry is one two-account
  pair with no link from a payment to the charge it settles, so a charge that
  was paid in full still lists as overdue until it is reversed. Matching a
  payment to a charge needs either a settles-entry reference on an entry or an
  invoice object with a settled amount; which is a product decision, so it is
  left open.
- **Ageing buckets** (0–30, 31–60, …) are the caller's: the read gives the
  days, and the buckets differ by domain.
- It reads one account. A read across a whole category would iterate the
  category's accounts, and was not needed for the row.
