# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project: mwanachama-backend-accounting

A domain-agnostic double-entry general ledger — chart of accounts, full
double-entry postings. Module path
`github.com/aosanya/mwanachama-backend-accounting`. Decided by the
dev-research session recorded at
[DSN-1698](../developer/documentation/2.%20design/todo.md).

Architected exactly like
[mwanachama-backend-taskmanager](../mwanachama-backend-taskmanager) and
[mwanachama-backend-actor](../mwanachama-backend-actor): a Go package on
`mwanachama-backend-shared`'s Postgres entity-graph store, imported directly
by `mwanachama-backend-api-gateway`. No gRPC, no standalone service, **no
Supabase, no RLS, no RPC** — every rule that used to live in a Postgres
policy or a `SECURITY DEFINER` function on the party plane is Go here, the
same retargeting `internal/domain/merchandise` already did for the stock
ledger.

**This repo never hardcodes a member, a paybill, a contribution, or any
other Mwanachama-specific concept anywhere an id or a vocabulary value
belongs** — the same discipline `mwanachama-backend-actor` holds for
"chapter" (`actor/CLAUDE.md`: "this repo never hardcodes the word 'Chapter'
anywhere an id belongs") and `mwanachama-backend-forms` holds for "survey".
`AccountKind`/`DocumentKind` are plain caller-defined strings this package
never validates against a fixed list; only `AccountType`
(asset/liability/equity/income/expense) is closed vocabulary, because
that's double-entry theory itself, not a decision any one domain gets to
make differently. An account's identity is `(Kind, HolderID)`; `HolderID`
is an opaque id the core never reads for meaning.

**Where Mwanachama's own vocabulary lives:** *not here*. Following the
`mwanachama-backend-forms` → `internal/store/formsadapter` precedent (the
gateway's own package translates its `Survey`/`Question`/`Answer`
vocabulary against `forms`'s generic `FormManager` — nothing
survey-specific exists inside `mwanachama-backend-forms` itself) and the
`mwanachama-backend-actor` → `ResourceNames`/gateway-literal-path precedent
for "chapter", the contribution domain's `AccountExternal`/`AccountBasket`/
`AccountSuspense`, the `AccountTypeFor` Kind→Type convention, and the
`DocContribution*` document kinds belong in
`mwanachama-backend-api-gateway`'s own `internal/store/accountingadapter`
(mirroring `formsadapter`'s shape), landing with DEV-1674. **A `contribution/`
subpackage briefly existed inside this repo on 2026-09-07 and was removed**
— it repeated exactly the "chapter"/"survey" coupling this note now warns
against, caught before DEV-1674 wired anything to it. See
`documentation/2. design/README.md`'s decisions for the *content* of that
vocabulary (still correct and still needed), which now belongs to whichever
repo builds `accountingadapter`, not to this one.

## Scope decisions (session of 2026-09-03, revised 2026-09-06/07)

- **Net-new general ledger, not an extraction.** The merchandise/stock
  ledger (`internal/domain/merchandise/ledger.go` in the gateway) stays
  exactly where it is. It closes on **held-stock**
  (`intake − outflows = Σ balances`); this ledger closes on **zero-sum
  currency** (every posting's two sides always net to zero). The two
  invariants are different enough that a shared core would be the wrong
  abstraction — they are separate systems by decision, not by omission.
- **Chart of accounts is org-wide for now** — no `chapter_id` on `Account`
  the way `merchandise_account` carries one. Extensible to per-tier accounts
  later; not designed in, and in any case a decision for whatever calls this
  package, not for this package.
- **One account per (Kind, HolderID).** A holder does not hold several
  accounts of the same kind; further context (which paybill, which purpose)
  comes from the *other side* of the entry, never from picking among
  several accounts for the same holder.

The specific account kinds (external/basket/suspense), their Kind→Type
pairing, and the statement-import suspense-account shape are Mwanachama's
own decisions — recorded in `documentation/2. design/README.md` for the
historical record, implemented in the gateway's `accountingadapter`, not
here.

## Open questions (flagged, not silently decided)

- **Access control.** No capability/authorization model belongs in this
  repo at all — it has no HTTP layer and no auth of any kind, by design.
  Mwanachama's own decision (an organization-wide, treasury-scoped
  `CapViewLedger`/`CapPostLedger` pair, not any HQ admin — see
  `documentation/2. design/README.md` decision 2) is enforced in
  `mwanachama-backend-api-gateway`'s `internal/api/http/capability.go`
  alongside `CapViewMerchandise`'s existing pattern, landing with DEV-1675.
- **Multi-currency.** Out of scope for this package — `Entry.Amount` has no
  currency field; a deployment needing more than one currency runs one
  ledger per currency.

## Conventions

- Task status lives on
  [documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
- Four-phase `documentation/` layout — see
  [documentation/README.md](documentation/README.md).
- Before wiring into `mwanachama-backend-api-gateway`, check
  `internal/domain/merchandise` there for naming/vocabulary overlap
  (`Account`, `Entry`, `Post`, `ReversesEntryID` are deliberately the same
  words — keep the two packages' types distinguishable by package name,
  never by renaming one of them to something worse).
- **A new domain wanting an `AccountKind`/`DocumentKind` value never edits
  this repo.** `Kind`/`DocumentKind` already take any string; a new
  consumer (a school ledger, a token ledger, Mwanachama's own
  contribution flow) defines its own vocabulary in its own calling code —
  a gateway-side adapter package, the way `formsadapter` does for surveys.
