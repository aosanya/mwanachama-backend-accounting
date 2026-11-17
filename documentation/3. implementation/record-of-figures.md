# Record of figures

A figure that is not money, such as a ratio or a score a calculating step works out, is kept in the record of figures. It sits beside the ledger and the ledger stays money only. Decision 7 of [compute-decisions.md](../../../mwanachama-backend-agency/documentation/1.%20requirements/compute-decisions.md): one entry per subject, figure and day, each carrying how it was worked out, append-only. Hub issue ISS-46.

Status: built and tested on the in-memory SQLite side, not reviewed by the owner. Every name and field below is proposed, except where a page says otherwise.

## The object

The blueprint declares the role `figure`. The shipped domain fills it as `figure_entry`, in the table `figure_entries` (proposed names; the store prefixes the instance and a hash to every table name, as it does for the other roles). Every field is written once and never changes.

| Field | What it holds | Written by |
|---|---|---|
| `id` | the storage key | the store |
| `subject_id` | who or what the figure is kept for, such as a member; an opaque id this module never reads for meaning | the caller |
| `figure` | the figure's name, lower-case letters, digits and underscores, starting with a letter, at most 63 characters, such as `repayment_ratio` | the caller |
| `on_day` | the day the figure is for, written `YYYY-MM-DD`, a day that exists | the caller |
| `value` | the figure as decimal text, at most 30 digits, kept as written (`45.20` stays `45.20`, `-0.00` is accepted) | the caller |
| `function_code` | the code of the function that worked it out | the caller |
| `function_version` | the published version of that function | the caller |
| `inputs_fingerprint` | a fingerprint of the inputs, kept as given | the caller |
| `actor_id` | who or what caused the entry, never only a role | the caller |
| `narration` | the sentence the record reads back; optional | the caller |
| `recorded_at` | when the entry was written | the store |

A unique index on `subject_id`, `figure` and `on_day` is declared in the blueprint. `spec.Migrate` makes it (checked in the migrated in-memory schema) and `provision.go` needed no change.

## Operations

| Operation | Address | Action | What it does |
|---|---|---|---|
| `append_figure` | `POST /figures` | `accounting.figure.append` | appends one entry and returns it with its id and time |
| `list_figure_history` | `GET /subjects/{subjectID}/figures/{figure}` | `accounting.figure.history` | the entries of one figure of one subject, newest day first, then newest written first; `limit` caps it, zero means the default page of 200 |

They are the last two in the operations file, so the by-index mount of the two consumers does not move. The repository interface gains `AppendFigure` and `ListFigureHistory`, and nothing else with `Figure` in its name: there is no update and no delete, in the operations or in the interface. A test holds both.

## What is refused

| Case | Answer |
|---|---|
| a second entry for the same subject, figure and day | `ErrFigureTaken`, 409: accounting: that subject already has an entry for this figure on this day. The first entry stands untouched. |
| no subject, figure, day, value, function code, function version, inputs fingerprint or actor | `ErrInvalid`, 400, naming the missing field |
| a figure name with a capital or a space, a day that does not exist or is written the other way round | `ErrInvalid`, 400 |
| a value that is not a decimal number: `1,200.50`, `95%`, `1e3`, `abc`, or more than 30 digits | `ErrInvalid`, 400 |
| a domain that does not fill the figure role | `ErrInvalid`, 400: this domain keeps no record of figures |

An existing instance spec that does not fill the role keeps working: the store takes the figure carrier only when the spec fills the role.

## What else changed

- `ledger_store.go`: six additive lines in `NewLedgerRepository`, the carriers map. The issue asked not to touch this file because other issues change it; they wait on this one, and the constructor lives here.
- `ledger_repository.go`: the two methods.
- `routes/routes.go` and the operations file's `errors` map: `ErrFigureTaken` is a 409.
- `mwanachama.accounting.json`: the shipped domain fills the role.
- The shape tests moved with the two new operations: six roles in `blueprint_test.go`, 19 routes in `routes/routes_test.go` and `routes/routes_cover_test.go`.
- The hub's copy of this module (`hub-copy-diff.md`) does not have any of this yet, so the diff script will report the files above until the change is carried over.

## Checked against the pages

[posting-step.md](../../../mwanachama-backend-agency/documentation/2.%20design/posting-step.md) section 11 and [held-figures.md](../../../mwanachama-backend-agency/documentation/2.%20design/held-figures.md) section 10, sentence by sentence.

Matches:

- One entry per subject, figure and date: the unique index and the refusal.
- Each entry carries how it was worked out: the function code, the published version and a fingerprint of the inputs, the three facts of posting-step.md section 3, all required.
- The ledger stays money only: a separate object, a separate table, a decimal text value and no account.
- The history of a figure can be read and each entry traced: the history read, newest first.
- Append-only: every field immutable, no update or delete.
- A released figure carries the provenance of the figure approved, so the reworked ones when reworked: the caller passes them, the store keeps what it is given.

What the pages leave open and this object chose (proposed):

- The record's fields beyond the three facts, who caused it and the day: the object adds `narration` and `recorded_at`.
- The value is decimal text and not minor units, because a figure is not an amount.
- The fingerprint is stored as given. Its shape is open (posting-step.md section 12, question 6), so nothing checks it beyond it being there.
- The day is the caller's. Held-figures.md section 11, question 7 proposes the day the figure was released; the store takes the day it is given and does not choose it.
- The held date: held-figures.md section 10 asks for the held date in the provenance, and the note "Reworked at approval; held 140.00 on Monday" is a sentence. The object has no field for the held date; the sentence goes in `narration`.

## Open

Copied from the pages, not decided:

1. The step that writes an entry to the record of figures, and the record's fields beyond those above (posting-step.md section 12, question 1).
2. Which day the entry of a held figure carries (held-figures.md section 11, question 7; proposed: the day it was released, with the held date in the provenance).
3. How a wrong entry is put right. One entry per subject, figure and day means a second entry for the day is refused, and no page says how a mistake is corrected: a reversing entry as in the ledger, a new day, or something else.
4. Where the held date lives if it is not the narration.
5. The shape of the fingerprint and whether the agency's publication tag travels with it (posting-step.md section 12, question 6).

## Not here

The posting step and the ledger entry, which need the audit's gaps ([accounting-ledger-audit.md](../../../mwanachama-backend-agency/documentation/3.%20implementation/accounting-ledger-audit.md)), and the host that calls these operations (`mwanachama-wakala-api`).
