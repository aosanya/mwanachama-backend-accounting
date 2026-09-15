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
pairing, describes vocabulary that belongs in
`mwanachama-backend-api-gateway`'s own `internal/store/accountingadapter`
(mirroring `internal/store/formsadapter`'s shape for surveys) — a
`contribution/` subpackage briefly existed inside *this* repo holding it
and was removed; the content is still correct, only its address changed.

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
doesn't repeat that. **Decided, not yet built**: the capability constants
and the enforcement itself live in `mwanachama-backend-api-gateway`'s
`internal/api/http/capability.go`, alongside `CapViewMerchandise`'s
existing pattern, and land with that repo's DEV-1674/1675 wiring, not in
this repo — this repo has no HTTP layer and no auth of any kind, by design
(see CLAUDE.md).

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
`MemoryRepository`/`PostgresRepository`, then removed and due to land
instead in `mwanachama-backend-api-gateway`'s `internal/store/
accountingadapter` — see `todo_done.md`'s W11/W13. **Still not built
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

| Field | Type | Notes |
| --- | --- | --- |
| `ID` | uuid | |
| `PostedAt` | timestamp | when it hit the books |
| `OccurredAt` | timestamp, nullable | when the payment happened; ordering key, coalesces to `PostedAt` |
| `FromAccountID` | uuid | |
| `ToAccountID` | uuid | |
| `Amount` | decimal | always positive; direction is the two accounts, never a sign |
| `DocumentKind` | string, open | this domain uses `accountingadapter.DocContribution` \| `DocContributionUnmatched` \| `DocContributionMatched` \| `DocCorrection` — the root package does not validate against a fixed list |
| `DocumentID` | uuid | polymorphic pointer at what caused the posting (a statement row, a manual attach, a correction) |
| `ActorID` | uuid | the person who caused the posting — accountability, same as `merchandise_entry.actor_id` |
| `Detail` | text | the sentence the ledger reads back |
| `ReversesEntryID` | uuid, nullable | corrections only — the original is never touched |

No balance column anywhere, by the same rule G49 states for merchandise: a
balance is a fold over entries, computed on read.
