# Days: day allowance and day entry

Days as a unit of work, kept beside the ledger and never in it. A day allowance and a day entry kept in the accounting module, with a days balance read.

Status: built and tested on the in-memory SQLite side, not reviewed by the owner. Every name and field below is proposed.

## The objects

The blueprint declares two roles. The shipped domain fills them as `day_allowance` (table `day_allowances`) and `day_entry` (table `day_entries`). A domain that fills neither refuses every days call with `ErrInvalid`. Days are whole hundredths of a day, so 100 is one day and 850 is 8.5 days. A rate is the fee for one day in minor units of the working currency.

| Role | Fields | Written once |
|---|---|---|
| `day_allowance` | `project`, `consultant`, `position`, `days_bought`, `rate`, `period_start`, `period_end`, `replaces_id`, `reason`, `set_by`, `set_at` | every field |
| `day_entry` | `project`, `consultant`, `worked_on`, `days`, `rate`, `description`, `status`, `submitted_by`, `submitted_at`, `decided_by`, `decided_at` | all but `status`, `rate`, `decided_by`, `decided_at` |

## The rules

- A consultant has one current allowance on a project. The current row is the one no other row names in `replaces_id`.
- A change of days is a new row that names the current row it replaces and a reason. The old row stays. A row that is not current cannot be replaced, and a second row for a consultant who already holds one is refused.
- An entry starts `submitted` and is decided once, to `approved` or `rejected`. Deciding again is `ErrDayEntryDecided` (409).
- Approving an entry whose days, added to the consultant's approved days, pass the days bought is `ErrOverDaysBought` (409). So is approving days for a consultant who holds no allowance on the project. The entry stays submitted.
- The consultant name is trimmed and written in lower case on every write, and every lookup and list filter is normalised the same way, so `Ana Lund`, `ana lund` and `Ana Lund ` are one person and a second allowance under another spelling is refused. A balance line shows the lower-case name. Rows written before this rule are not rewritten.
- Approving an entry stamps it with the rate of the allowance in force at that moment. A replacing allowance prices only days approved after it; days already approved keep their own rate.
- A replacing allowance whose days are fewer than the consultant's approved days is `ErrOverDaysBought` (409).
- The person approving an entry is not the person who submitted it or the consultant it names, compared after trimming and lower-casing. Otherwise `ErrOwnDayEntry` (409). Rejecting is open to the submitter, so a person can withdraw their own entry.
- An entry is at most one day (100 hundredths), else `ErrInvalid`. The non-rejected entries of one consultant on one project and date add up to at most one day, else `ErrDateOverfull` (409). A rejected entry frees its date. A month of work is therefore a run of daily entries, and a double submit is refused.
- An allowance whose days times rate would not fit a whole number is `ErrInvalid`.
- `period_start` and `period_end` are kept and checked as an RFC 3339 pair. They do not limit which dates an entry may carry.

## The days balance

`DaysBalance(project)` returns one line per consultant and a total. Bought is the current allowance's days. Submitted is the days of entries still waiting for a decision. Approved is spent. Left is bought minus approved. Fee is the sum, over the rates days were approved at, of those days at that rate, rounded half up to a whole minor unit. Fee left is the days left at the current allowance's rate, and the fee ceiling is the fee plus the fee left, so a new rate changes the ceiling only for the days still to come. Entries approved before the rate column existed carry a rate of zero. A consultant with entries and no allowance still has a line, with nothing bought.

## Operations

| Operation | Address | Action |
|---|---|---|
| `add_day_allowance` | `POST /day-allowances` | `accounting.day_allowance.create` |
| `replace_day_allowance` | `POST /day-allowances/{allowanceID}/replace` | `accounting.day_allowance.replace` |
| `list_day_allowances` | `GET /projects/{project}/day-allowances` | `accounting.day_allowance.list` |
| `submit_day_entry` | `POST /day-entries` | `accounting.day_entry.submit` |
| `approve_day_entry` | `POST /day-entries/{entryID}/approve` | `accounting.day_entry.approve` |
| `reject_day_entry` | `POST /day-entries/{entryID}/reject` | `accounting.day_entry.reject` |
| `list_day_entries` | `GET /projects/{project}/day-entries` | `accounting.day_entry.list` |
| `days_balance` | `GET /projects/{project}/days-balance` | `accounting.days.balance` |

Who set, submitted or decided comes from the mount's caller, never from the request.

## Tests

`day_store_test.go` holds the worked example of section 5 of days.md (130 days bought, 101.0 spent, 29.0 left, fee 63,850, fee left 20,150), the refusal past the days bought and the approval after a replacing row, the replacement rules, the once-only decision, and a domain that fills no day role. The worked example is entered as daily entries. `day_findings_test.go` holds one test for each of: a new rate leaving approved days at their rate, a replacement below the approved days, approval by the submitter, the one-day-per-date bound, the name spellings, and the fee overflow. `routes/day_routes_test.go` runs the same refusal over HTTP and checks the caller comes from the mount. The role list and the route list in `blueprint_test.go`, `routes/routes_test.go` and `routes/routes_cover_test.go` follow the two new roles and the eight new routes.

## What carries to the hub

The hub's copy of this module has no days yet and has drifted from this one in both directions (`hub-copy-diff.md`), so no file of the hub copy can be replaced whole. What a custodian carries across, with the import rewrite the diff script applies:

- whole files: `day_store.go`, `models/day.go`, `day_store_test.go`, `routes/day_routes_test.go`.
- merged into files the hub also changed: the two roles at the end of `accounting.blueprint.json`, the eight operations and the two errors in `accounting.operations.json`, the two objects in `mwanachama.accounting.json`, `DayRepository` in `LedgerRepository`, the `addDayCarriers` line in `NewLedgerRepository`, the two sentinels in `routes/routes.go`, and the pinned role and route counts in the tests.
- hub only, not in this repository: the served-tool list in the hub's `routes/tools.go` filters by action prefix, so `accounting.day_allowance.`, `accounting.day_entry.` and `accounting.days.` need adding for the tools to reach the MCP roster, and the permission catalogue and its test in `wakala-api` need the eight actions.
