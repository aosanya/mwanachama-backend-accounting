# Design

The chart-of-accounts / double-entry schema, and how it maps onto
`mwanachama-backend-shared`'s entity-graph store — the same retargeting
`mwanachama-backend-taskmanager` and `internal/domain/merchandise` (in the
gateway) already did for their own domains.

This file records the decisions from the dev-research session of
2026-09-03 ([DSN-1698](../../../developer/documentation/2.%20design/todo.md))
and the open questions it did not settle, so neither has to be re-derived
from a chat transcript.

**2026-09-07 — read this file as a historical record of decisions this
repo's own code no longer holds.** This package (`accounting`) is a
domain-agnostic ledger core (`Account{ID, Kind, HolderID, Type, ...}`,
`Entry{...}`) that does not know what a member, a paybill, or a contribution
is — the same discipline `mwanachama-backend-actor` holds for "chapter" and
`mwanachama-backend-forms` holds for "survey" (see `CLAUDE.md`). Everything
below that names `member`/`basket`/`suspense`, or an `AccountTypeFor`-style
pairing, describes vocabulary that lives with the consumer, not here — a
`contribution/` subpackage briefly existed inside *this* repo holding it
and was removed; the content is still correct, only its address changed.

**2026-09-28 — that address has changed again, and this file was written
before it did.** Every `accountingadapter.X` name below should be read as
"the consumer's own name for X". Two consumers exist:

- **`mwanachama-wakala-api`, run locally, is the working one.** It mounts
  this repo's declared routes per registered instance, and a domain's
  categories and document kinds are **declared data in that instance's
  `accounting.<domain>.json` spec** — not Go constants in an adapter
  package. The
  [party-mobilization template](../../../developer/documentation/4.%20qa/agencies/party-mobilization/accounting.json)
  is a worked example. Because each instance owns its own table set, the
  "org-wide chart of accounts" decision below is bounded by the instance
  there.
- **`mwanachama-backend-api-gateway`** still imports this repo and still
  holds `internal/store/accountingadapter`, which is what the names below
  literally refer to. That path is older and single-tenant; it is not what
  current work targets.

Everything else below — the suspense-account shape, the Kind→Type pairing,
why this is not the merchandise ledger generalized — is unaffected by which
consumer holds the vocabulary.

## Why a new ledger, and why it isn't the merchandise ledger generalized

The merchandise ledger (`merchandise-account.md` /
`merchandise-entry.md`, ported into the gateway as
`internal/domain/merchandise/ledger.go`) already proves the pattern this
repo reuses: keep entries, not balances (G49); append-only, corrected only
by a new entry naming what it reverses; no nullable "other side" of a
posting. It was tempting to generalize that code directly. It was decided
**not to**, for one reason that matters more than the shared vocabulary:
the two ledgers close on different invariants.

- **Merchandise** closes on **held-stock**:
  `intake − outflows = Σ balances`. A `loss` account and an `in_transit`
  account exist so that every posting always has two real sides, but the
  organization's total stock is not required to net to zero against
  anything outside itself — stock simply *is somewhere*.
- **Money**, under full accounting, must close on a real **zero-sum
  double-entry equation** — every posting's debit and credit are the same
  amount, and a trial balance across every account must net to zero. That
  is a stronger, differently-shaped guarantee, and bolting it onto
  `merchandise.Account`/`merchandise.Entry` would have meant carrying two
  incompatible closure rules behind one type.

So: a new repo, a new `Account`/`Entry` pair, and the merchandise ledger
stays exactly where it is.

## Chart of accounts

**Org-wide, for now.** Unlike `merchandise_account`, an `Account` here
carries no `chapter_id` — there is one set of books for the whole
organization. This is a deliberate simplification, not a permanent
architectural stance: as the platform rolls out, other accounts (expense
categories, per-tier sub-ledgers) may be added, but that scoping is not
designed into the schema yet and should not be assumed.

**Account kinds, decided in session** (now `accountingadapter.AccountExternal`/
`accountingadapter.AccountBasket`, plus `accountingadapter.AccountSuspense` added
2026-09-06/07 — decision 4 below):

- **`external`** — one per member (today's only external party — see
  `accountingadapter.AccountExternal`'s doc comment for why the type itself
  doesn't say "member"). **Lazily opened**, the same rule
  `merchandise_account` uses (G235): an account exists because something
  was posted to it, never because a member registered. A member who has
  never contributed has no account row, and that is a different state from
  an account with a zero balance (same distinction `merchandise-account.md`
  draws between *never traded* and *traded to zero*).
- **`basket`** — one per paybill. An organization may open more than one
  M-Pesa paybill; each gets its own basket account, independently
  queryable. A contribution posts `Debit: basket / Credit: external`.

**A member has exactly one account.** An earlier direction in the research
session considered a member holding several accounts (an address-book
analogy — home address, work address). That was walked back: a
contribution's context (which paybill it came through, what it was for) is
read off **the other side of the entry** — the basket it posted against —
never by picking among several accounts belonging to the same member. This
mirrors the merchandise ledger's own resolved argument that a movement's
detail belongs to the pairing, not to a flag on one side.

## Decisions closed 2026-09-06/07

**1. What account type is a member's account? → `income`, confirmed.**
Crediting a member's account on a contribution recognizes contribution
income; the org-wide books carry `basket` (asset), `external` (income), and
`suspense` (liability — decision 4 below) without contradiction. This is
now encoded in code, not just assumed: an `AccountTypeFor(kind)`-style
function in `accountingadapter` is the one place this domain's kind→type
pairing is decided, and
`accountingadapter`'s own OpenAccount-style helper fills in `Type` from it so the pairing can't be
gotten wrong at a call site the way an unconfirmed assumption could have
been. **Not enforced by the ledger core itself** — see `CLAUDE.md`'s "Why
the split": a different domain built on the same core may pair its own
kinds differently, so this discipline lives in `accountingadapter`, not in
`Account.Validate()`.

**2. Access control → an organization-wide, treasury-scoped capability
pair.** Not chapter-ancestor-walked (there is no chapter to scope
against — see "org-wide" above): `CapViewLedger`/`CapPostLedger`, held by an
operator carrying a treasury role specifically, not any HQ admin. Chosen
over "any HQ admin" because the contribution board's own history
(`todo_contributions_match.md`'s DEV-907) already flagged `is_admin()`
gating a money-read as too broad on the old Supabase plane — this ledger
doesn't repeat that. The capability constants and the enforcement itself
were to live in `mwanachama-backend-api-gateway`'s
`internal/api/http/capability.go`, alongside `CapViewMerchandise`'s
existing pattern, and land with that repo's DEV-1674/1675 wiring, not in
this repo — this repo has no auth of any kind, by design (see CLAUDE.md).

**2026-09-28 — superseded on the working path.** `mwanachama-wakala-api`
gates accounting through `mwanachama-backend-permissions` on the `action`
each declared operation carries (`accounting.account.open`,
`accounting.entry.post`, …) under a `module:accounting` scope, passed in as
`routes.Mount{Authorize: …}`. That is per-operation, where
`CapViewLedger`/`CapPostLedger` was a read/write pair, so the treasury-scoped
intent above still has to be expressed as a set of actions granted to a
treasury role — it does not carry over by itself. This repo did gain a
`routes/` package (W14) since this decision was written, but it still holds
no auth: `Authorize` is supplied by the caller.

**3. Multi-currency.** Still deferred. `contribution.currency` today is "one
currency per organization" (`contribution.md`); this ledger inherits that
assumption — `Entry.Amount` carries no currency code yet.

**4. The statement-import / phone-hash-match mechanism → a `suspense`
account per basket, not a one-sided entry.** The naive first read of this
question ("an unclaimed statement row is an entry posted only against the
basket, no member side yet") is exactly the one-sided posting this ledger's
own zero-sum design rules out. Resolved instead the same way the
merchandise ledger's `in_transit` account resolves an analogous problem
(money/stock that has left one side with no confirmed second side yet):

- **Import, unmatched or orphaned**: `Debit: basket / Credit: suspense`
  (`accountingadapter.DocContributionUnmatched`). `unclaimed` vs. `orphaned`
  stays a *derived* read exactly as `internal/domain/contribution.Status`
  already computes it — which side of the live salt version the row's hash
  falls on, not a stored value; nothing here needs to know which of the two
  it's looking at in order to post correctly.
- **Import, matched at the point of import** (the hash already resolves to
  a member): `Debit: basket / Credit: external` directly
  (`accountingadapter.DocContribution`, unchanged) — no need to round-trip
  through suspense when the second side is already known.
- **Later match or manual attach**: `Debit: suspense / Credit: external`
  (`accountingadapter.DocContributionMatched`), `DocumentID` pointing at the
  `DocContributionUnmatched` entry it resolves. The import entry is never
  touched — a correction is still always a new entry with `ReversesEntryID`
  set.
- **Why per-basket, not one global suspense account**: keeps each paybill's
  unmatched total independently reconcilable, the same reason a basket
  itself is per-paybill rather than one org-wide cash account.

Prototyped once as `contribution/contribution.go` inside this repo
(`AccountSuspense`, `DocContributionUnmatched`, `DocContributionMatched`,
`AccountTypeFor`, `OpenAccount`), proven end to end against this repo's own
`MemoryRepository`/`PostgresRepository`, then removed — see `todo_done.md`'s
W11/W13. It landed in `mwanachama-backend-api-gateway`'s
`internal/store/accountingadapter`; on the wakala-api path the same three
categories and document kinds are declared in the instance's
`accounting.<domain>.json` spec instead. **Still not built
anywhere**: the statement parser, the phone-hash
computation and matching itself, and dedup on
`(statement_import_id, external_ref)` — those need
`StatementImport`/`PhoneSalt`-equivalent types this repo doesn't have yet,
which is a separate, larger port than the ledger-posting shape this
decision closes.

## Fields (as built — `Account`)

The root package's actual shape — generic, not contribution-specific. Where
this domain's usage fixes a value, it's noted alongside.

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | uuid | |
| `Kind` | string, open | this domain uses `accountingadapter.AccountExternal` \| `AccountBasket` \| `AccountSuspense` — the root package does not validate against a fixed list |
| `HolderID` | string, opaque | a member id for `AccountExternal`, a paybill id for `AccountBasket`/`AccountSuspense`; identity is `(Kind, HolderID)` together |
| `Type` | enum | asset / liability / equity / income / expense — this domain pairs one fixed value per `Kind` via `accountingadapter.AccountTypeFor` (or an equivalent helper), but the root package does not enforce that pairing (see `CLAUDE.md`'s "Project" section) |
| `OpenedAt` | timestamp | set on first posting (lazy open, G235) |
| `ClosedAt` | timestamp, nullable | mirrors merchandise: refused while balance ≠ 0 |

## Fields (as built — `Entry`)

Renamed to standard double-entry vocabulary on 2026-09-28, and declared in
`accounting.blueprint.json` rather than in Go. Timestamps are RFC 3339
strings at nanosecond, fixed-width precision (`models.TimeLayout`), because
they are stored as text and ordered lexicographically.

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | storage key |
| `PostedAt` | timestamp | when it hit the books |
| `OccurredAt` | timestamp, optional | when the payment happened; ordering key, coalesces to `PostedAt` |
| `DebitAccountID` | string | the receiving side — was `FromAccountID`'s counterpart, `ToAccountID` |
| `CreditAccountID` | string | the source side — was `FromAccountID` |
| `Amount` | int64 | always positive, in the smallest denominated unit; which side is which is the two accounts, never a sign |
| `DocumentKind` | string, open | the source document's kind in the calling domain's words — an invoice, a receipt, a statement import. The module enforces that every entry names one, never which ones exist |
| `DocumentID` | string | pointer at what caused the posting |
| `ActorID` | string | who caused the posting — accountability, same as `merchandise_entry.actor_id` |
| `Narration` | text | the sentence the ledger reads back — was `Detail` |
| `ReversesEntryID` | string, optional | corrections only; the original is never touched, and an entry may be reversed **at most once** |

## Fields (as built — `Account`)

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | string | storage key |
| `Code` | string, optional | the chart-of-accounts number, e.g. `1000`. Caller-supplied, never minted here; unique across the ledger when given |
| `Name` | string, optional | the account's title as a ledger prints it |
| `Category` | string, open | what sort of account this is, in the domain's words — was `Kind` |
| `Type` | enum | asset / liability / equity / income / expense; the one closed vocabulary |
| `HolderID` | string | the subject this subsidiary account is kept for. Keeps its name: no bookkeeping term means this |
| `OpenedAt` | timestamp | written by the store |
| `ClosedAt` | timestamp, optional | the only value on an account that ever changes |

An account's identity is the unique index on `(category, holder_id)`.

### Which side is the debit

Money moves **into** the debit account. `Balance` folds
`debits − credits`, which is the standard debit-balance reading and is
numerically identical to what the entity-graph ledger computed as
`to − from` — so the conversion moved no balances. A consumer that reads an
account in its *natural* sense still negates for a credit-normal type; that
is the consumer's job, not this module's, because nothing here folds a
balance by `Type`.

### Two rules the spec cannot state, so they stay in Go

- **An entry may be reversed at most once.** `Post` checks inside the
  writing transaction and answers `ErrAlreadyReversed`; `Provision` also
  creates a partial unique index on `reverses_entry_id` where it is
  non-empty, so a true race is refused by the database rather than
  silently drifting the ledger off zero-sum. A plain declared `unique`
  would not do: every non-reversing entry stores `''`, not NULL, and they
  would all collide.
- **A non-empty `code` is unique.** Same shape, same reason —
  `ErrCodeTaken`, backed by a partial unique index.

No balance column anywhere, by the same rule G49 states for merchandise: a
balance is a fold over entries, computed on read.
