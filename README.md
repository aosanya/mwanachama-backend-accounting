# mwanachama-backend-accounting

A book-of-record general ledger — chart of accounts, full double-entry
postings — for [mwanachama-frontend-kazi](../mwanachama-frontend-kazi) and
the wider Mwanachama platform. Absorbs the `contribution` domain: a
contribution stops being a mirror of an outside paybill statement and
becomes a real ledger posting.

No gRPC, no sub-service shape, no Supabase. A **declared domain** as of
2026-09-28 (W16–W22): the objects are data, not Go row structs —
`accounting.blueprint.json` declares them, a domain spec names their
tables, and `spec.Migrate` is the whole storage story. Built on
[mwanachama-backend-shared](../mwanachama-backend-shared)'s
`spec`/`specstore`/`dispatch` engine and imported directly by its
consumer. See
[declared-domains.md](../mwanachama-backend-shared/documentation/2.%20design/declared-domains.md).

**The working consumer is
[mwanachama-wakala-api](../mwanachama-wakala-api), run locally.** It mounts
this repo's declared route table under
`/instances/{instanceID}/accounting`, resolving one `LedgerRepository` per
registered instance against that instance's own table set — so the ledger
is multi-tenant there, not org-wide-singleton.
[mwanachama-backend-api-gateway](../mwanachama-backend-api-gateway) also
imports this repo (W9) and keeps an `internal/store/accountingadapter`
holding Mwanachama's contribution vocabulary in Go; that wiring is the
older, single-tenant path and is not what current work targets.

**The field names are standard double-entry**, renamed on 2026-09-28: an
entry carries a `debit_account_id`, a `credit_account_id` and a
`narration`; an account carries a `code`, a `name` and a `category`. Money
moves *into* the debit account, so `Balance` folds `debits − credits`.
Anything written before that date uses `from_account_id`/`to_account_id`/
`detail`/`kind` — see [CLAUDE.md](CLAUDE.md) for the full mapping.

Every `Entry` is exactly one two-account pair, which is what makes the
zero-sum invariant structural. There is no multi-line Journal.

The merchandise/stock ledger (`internal/domain/merchandise`, in the
gateway) is a separate system and stays there: it closes on held-stock,
this ledger closes on zero-sum currency, and the two never merge.

See [documentation/](documentation/) for design and task board.
