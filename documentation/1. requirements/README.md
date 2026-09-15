# Requirements

## Problem

Mwanachama needs to keep money the way it already keeps stock: as a real
ledger, not a spreadsheet total. Today the only monetary object in the
platform, `contribution`
(`../../../developer/documentation/2. design/datamodel/contribution.md`),
is explicitly **not** a ledger — it mirrors an outside paybill statement or
cash book, joined to a member by hashed phone, and never claims to be the
organization's own record of what it holds.

## Vision

A book-of-record general ledger: a chart of accounts and double-entry
postings, so that every shilling the organization is told about has a
traceable, append-only, zero-sum-provable trail — the same guarantee the
merchandise ledger already gives stock (G49: entries, not balances), applied
to money.

## Scope (session of 2026-09-03 — see [DSN-1698](../../../developer/documentation/2.%20design/todo.md))

**In scope:**
- Chart of accounts, org-wide for now.
- Double-entry account/entry primitives: append-only entries, corrections
  by reversal (`ReversesEntryID`), balances computed as folds — never
  stored.
- One ledger account per member.
- One ledger account per paybill ("basket"), an organization may open more
  than one.
- The `contribution` domain absorbed as a real ledger transaction: a
  payment posts `Debit: paybill basket / Credit: member account`, and the
  statement-import / phone-hash-match mechanism that currently lives in
  Supabase RPC+RLS is reimplemented here as Go/Postgres logic.

**Explicitly out of scope:**
- The merchandise/stock ledger (`internal/domain/merchandise` in
  `mwanachama-backend-api-gateway`) — a separate system, closing on a
  different invariant (held-stock vs. zero-sum currency), never merged into
  this one.
- Per-tier (chapter-scoped) accounts — the chart of accounts is org-wide
  until a real need for tier scoping is decided.
- Multi-currency.

**Not yet decided, flagged rather than assumed (see
[2. design/README.md](../2.%20design/README.md)):**
- Which of the five standard account types (asset / liability / equity /
  income / expense) a member's account is.
- The access-control / capability model for who may post or view.
