# mwanachama-backend-accounting

A book-of-record general ledger — chart of accounts, full double-entry
postings — for [mwanachama-frontend-kazi](../mwanachama-frontend-kazi) and
the wider Mwanachama platform. Absorbs the `contribution` domain: a
contribution stops being a mirror of an outside paybill statement and
becomes a real ledger posting.

No gRPC, no sub-service shape, no Supabase. Built on
[mwanachama-backend-shared](../mwanachama-backend-shared)'s Postgres
entity-graph store and imported directly by
[mwanachama-backend-api-gateway](../mwanachama-backend-api-gateway) — the
same architecture as
[mwanachama-backend-taskmanager](../mwanachama-backend-taskmanager).

The merchandise/stock ledger (`internal/domain/merchandise` in the gateway)
is a separate system and stays there: it closes on held-stock, this ledger
closes on zero-sum currency, and the two never merge.

See [documentation/](documentation/) for design and task board.
