# mwanachama-backend-accounting (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in
[todo_done.md](todo_done.md).

| Task | Title | Status | Notes |
|------|-------|--------|-------|
| W12 | Port the statement parser and phone-hash-match mechanism itself (`contribution.md`, `statement-import.md`): dedup on `(statement_import_id, external_ref)`, salt rotation orphaning rather than re-hashing — reimplemented as Go/Postgres, no Supabase RPC or RLS | ⏸️ Blocked | The ledger-posting shape this needs (a `suspense` account per basket, `unclaimed`/`matched`/`orphaned` derived exactly as `internal/domain/contribution.Status` computes it today) is decided; the code proving it out was briefly built inside this repo as `contribution/`, then removed (W13) since it belongs in the gateway's `internal/store/accountingadapter`, not here — see [todo_done.md](todo_done.md)'s W10/W11/W13. What's left needs `StatementImport`/`PhoneSalt`-equivalent types neither this repo nor the gateway has yet — a real port, not a design gap. Should probably be its own sub-board once scoped, and lands alongside/after W9. |
