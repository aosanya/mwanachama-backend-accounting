# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project: mwanachama-backend-accounting

A domain-agnostic double-entry general ledger — chart of accounts, full
double-entry postings. Module path
`github.com/aosanya/mwanachama-backend-accounting`. Decided by the
dev-research session recorded at
[DSN-1698](../developer/documentation/2.%20design/todo.md).

**Storage is a declared domain as of 2026-09-28 (W16–W22).** The objects
are data, not Go row structs: `accounting.blueprint.json` declares the two
roles and every field, a domain spec names which table each lands in, and
`spec.Migrate` is the whole storage story — there is no `AutoMigrate` and
there are no row structs. This repo converted straight from
`mwanachama-backend-shared`'s entity-graph store to that format and never
had a GORM row-struct stage, which is what W16 decided; `schema.go`,
`postgres.go` and the hand-rolled `memory.go` are all gone. The reference
is [declared-domains.md](../mwanachama-backend-shared/documentation/2.%20design/declared-domains.md),
and `mwanachama-backend-catalog` is the repo whose shape this one follows.

No gRPC, no standalone service, **no Supabase, no RLS, no RPC** — every
rule that used to live in a Postgres policy or a `SECURITY DEFINER`
function on the party plane is Go here, the same retargeting
`internal/domain/merchandise` already did for the stock ledger.

## Who consumes this repo

**`mwanachama-wakala-api`, run locally, is the working consumer** — assume
it, not the gateway, when a task says "the consumer". It mounts this repo's
declared route table at `/instances/{instanceID}/accounting`
(`internal/api/http/accounting_routes.go`), resolving a `LedgerRepository`
per registered instance through `internal/agencies`' `Registry`, and gates
every route through `mwanachama-backend-permissions` on the declared
`action` rather than on a hand-written capability constant.

Two consequences that older notes in this repo predate:

- **The ledger is multi-tenant there.** Each instance owns its own table
  set, so "the chart of accounts is org-wide" below means *per instance*,
  and the single-tenant assumption `PostgresRepository` inherited has to be
  retired deliberately — see `todo.md`'s W18.
- **There is no `accountingadapter` on that path.** A domain's account
  categories and document kinds are declared data in that instance's
  `accounting.<domain>.json` spec, not Go constants in a caller's adapter
  package.

`mwanachama-backend-api-gateway` also imports this repo (W9) and does keep
`internal/store/accountingadapter`. That path is real but older and
single-tenant; it is not what current work targets, and nothing here should
be changed to suit it without saying so.

**This repo never hardcodes a member, a paybill, a contribution, or any
other Mwanachama-specific concept anywhere an id or a vocabulary value
belongs** — the same discipline `mwanachama-backend-actor` holds for
"chapter" (`actor/CLAUDE.md`: "this repo never hardcodes the word 'Chapter'
anywhere an id belongs") and `mwanachama-backend-forms` holds for "survey".
`AccountCategory`/`DocumentKind` are plain caller-defined strings this
package never validates against a fixed list; only `AccountType`
(asset/liability/equity/income/expense) is closed vocabulary, because
that's double-entry theory itself, not a decision any one domain gets to
make differently — and it is declared as an `enum` in the blueprint, with
`blueprint_test.go` holding the declared values and the Go constants to
each other in both directions. An account's identity is
`(Category, HolderID)`; `HolderID` is an opaque id the core never reads for
meaning.

## The vocabulary is standard double-entry (renamed 2026-09-28)

The field names are bookkeeping's, not this repo's own coinages. Anything
written before this date uses the old ones:

| Was | Is | Why |
|---|---|---|
| `Account.Kind` | `Account.Category` | `category` is the chart-of-accounts word for what groups accounts |
| `Entry.ToAccountID` | `Entry.DebitAccountID` | the receiving side of a posting is the debit |
| `Entry.FromAccountID` | `Entry.CreditAccountID` | the source side is the credit |
| `Entry.Detail` | `Entry.Narration` | a journal entry's explanatory line is its narration |
| — | `Account.Code`, `Account.Name` | a real chart of accounts numbers and names its accounts |

**Get the two sides the right way round.** Money moves *into* the debit
account, so `Balance` folds `debits − credits` and reads as the standard
debit balance. `HolderID` kept its name on purpose: it is what makes an
account a subsidiary-ledger account, and no bookkeeping term means that.

`Account.Code` is optional and unique only when given. The spec cannot
declare that — `specstore` writes an absent string as `''` rather than
NULL, so one column-level `unique` would collide across every codeless
account — so `Provision` creates a partial unique index and `OpenAccount`
answers `ErrCodeTaken`.

**There is no Journal.** Every `Entry` is exactly one two-account pair,
which is what makes the zero-sum invariant structural rather than a rule
something has to enforce. A multi-line balanced journal is filed as W23,
not built.

**Where Mwanachama's own vocabulary lives:** *not here* — and on the
wakala-api path, not in Go at all.

The contribution domain's `AccountExternal`/`AccountBasket`/
`AccountSuspense`, the `AccountTypeFor` Kind→Type convention, and the
`DocContribution*` document kinds are **one instance's declared data**:
they belong in that instance's `accounting.<domain>.json` spec, which
carries its own categories the way the
[party-mobilization template](../developer/documentation/4.%20qa/agencies/party-mobilization/accounting.json)
already does. A school ledger and a sacco declare different categories over
the same core and neither forks Go code.

On the older gateway path the same vocabulary lives instead in
`mwanachama-backend-api-gateway`'s `internal/store/accountingadapter`,
following the `mwanachama-backend-forms` → `internal/store/formsadapter`
precedent (the gateway's own package translates its
`Survey`/`Question`/`Answer` vocabulary against `forms`'s generic
`FormManager` — nothing survey-specific exists inside
`mwanachama-backend-forms` itself) and the `mwanachama-backend-actor` →
`ResourceNames` precedent for "chapter". That package exists and works; it
is simply not where the wakala-api deployment reads its vocabulary from.

**A `contribution/` subpackage briefly existed inside this repo on
2026-09-07 and was removed** — it repeated exactly the "chapter"/"survey"
coupling this note warns against. See `documentation/2. design/README.md`'s
decisions for the *content* of that vocabulary (still correct and still
needed), which belongs to a consumer or an instance spec, never to this
repo.

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
  package, not for this package. On wakala-api "org-wide" is bounded by the
  instance, since each instance holds its own table set.
- **One account per (Kind, HolderID).** A holder does not hold several
  accounts of the same kind; further context (which paybill, which purpose)
  comes from the *other side* of the entry, never from picking among
  several accounts for the same holder.

The specific account kinds (external/basket/suspense), their Kind→Type
pairing, and the statement-import suspense-account shape are Mwanachama's
own decisions — recorded in `documentation/2. design/README.md` for the
historical record, and held by the consumer: an instance's
`accounting.<domain>.json` spec on the wakala-api path, the gateway's
`accountingadapter` on the older one. Never here.

## Open questions (flagged, not silently decided)

- **Access control.** No capability/authorization model belongs in this
  repo at all — it has no auth of any kind, by design. The declared
  operations each carry an `action` (`accounting.account.open`,
  `accounting.entry.post`, …) and `routes.Mount` takes an `Authorizer`; who
  may call what is entirely the consumer's answer. On wakala-api that is
  `mwanachama-backend-permissions`, gating on the declared action under the
  `module:accounting` scope. The older gateway path instead enforces
  Mwanachama's own treasury-scoped `CapViewLedger`/`CapPostLedger` decision
  (see `documentation/2. design/README.md` decision 2) in
  `internal/api/http/capability.go`. Note the two differ: the declared
  actions are per-operation, the capability pair is read/write — reconcile
  deliberately rather than assuming they line up.
- **Multi-currency.** Out of scope for this package — `Entry.Amount` has no
  currency field; a deployment needing more than one currency runs one
  ledger per currency.

## Conventions

- Task status lives on
  [documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
- Four-phase `documentation/` layout — see
  [documentation/README.md](documentation/README.md).
- Changing a route address or an `action` in
  `accounting.operations.json` breaks **both** consumers — wakala-api's
  `accounting_routes.go` mounts `Shape()` by index, and the gateway mounts
  its own. Check both before moving an address.
- When wiring into `mwanachama-backend-api-gateway`, check
  `internal/domain/merchandise` there for naming/vocabulary overlap
  (`Account`, `Entry`, `Post`, `ReversesEntryID` are deliberately the same
  words — keep the two packages' types distinguishable by package name,
  never by renaming one of them to something worse).
- **A new domain wanting a `category`/`document_kind` value never edits
  this repo.** Both already take any string; a new consumer (a school
  ledger, a token ledger, Mwanachama's own contribution flow) declares its
  own vocabulary in its own instance spec, or in its own adapter package
  the way `formsadapter` does for surveys.

## Code comments

Write code with no comments. Not one-liners above a function, not section
banners, not doc comments on exported symbols, not "why" notes next to a
tricky line. A name, a type, or a smaller function carries it instead.

Anything that genuinely needs explaining goes in this repo's `documentation/`
folder, under the phase it belongs to (`1. requirements`, `2. design`,
`3. implementation`, `4. qa`) — never inline.

**Why:** inline prose drifts out of sync with the code, duplicates what
`documentation/` already owns, and buries the explanation where nobody
looking for it will search.

**How to apply:**

- New code ships without comments. If a line seems to need one, rename or
  split until it doesn't.
- Touching code that already has comments: strip the ones in the code you are
  changing. Do not sweep untouched files unless asked.
- If the reasoning matters, add or update the matching `documentation/` page
  in the same change and leave nothing behind in the source.
- Machine-read directives are not comments and stay: build tags, `//go:embed`,
  `//go:generate`, linter pragmas (`//nolint`, `// eslint-disable-next-line`,
  `// ignore:`), license headers, codegen "do not edit" banners, and generated
  files as a whole.
- Commit messages, PR descriptions, and test names carry the narration that
  used to go in comments.

This rule is repeated verbatim in every mwanachama repo's `CLAUDE.md` so that
it reaches sessions that do not load this machine's user-level config —
scheduled cloud routines, other machines, and other agent harnesses.
