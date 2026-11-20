# Project on a bill and on an upward report

A bill and an upward report can name the project they concern. Both fields are optional, and a bill or report sent without one reads as it did before.

Status: built and tested on the in-memory SQLite side, not reviewed by the owner.

## On a bill

| Field | Holds | Checked |
|---|---|---|
| `project` | The project key the bill is raised for | Trimmed; a valid dimension value (at most 200 characters, no control character), else `ErrInvalid` and nothing is stored |

The value is stored as the dimension `project` on the bill's entry, the same key fee entries carry. Three things follow:

- The bill and the bill list show it, and `list_bills` narrows by `project`.
- `dimension_totals` with key `project` and account type `asset` gives what was invoiced per project (the receivable side); with account type `income` it gives the same amounts negated. Without an account type every entry nets to zero.
- A bill correction replaces the dimensions as sent, so a correction that leaves `project` out clears it, as it does for the payer links.

Payments are not changed: a payment entry does not carry the bill's project, so cash received is not totalled per project by this change.

## On an upward report

| Field | Holds | Checked |
|---|---|---|
| `project` | The project the report was cut for, kept as sent | Trimmed; a valid dimension value, else `ErrInvalid` |

With a project, the report folds only the budget lines of the period whose `department` is that project, and each line's spent counts only entries on the line's account that carry the dimension `project` with that value. Planned, spent, difference and the department figures come from those lines. A project with no budget line of that period is `ErrNotFound`, and nothing is stored. Without a project the whole period is folded as before.

This leans on one convention, set in the budget design: the department of a budget line carries the project code. No field was added to the budget line.

`list_upward_reports` narrows by `project`, and by `recipient_record_row_id` and `project` together. The blueprint indexes the column.

## What this does not do

- The period of a report must still equal the period of its budget lines, exactly.
- A project funded by two donors gives the same figures in each donor's report; the figures are not split between the donors.
- The report read (`get_report`) is not cut by project; only the sent record is.
- Amounts carry no currency.

## Tests

`project_bill_test.go` and `project_report_test.go` hold the store cases, and `routes/project_routes_test.go` the HTTP cases. Each fails when the store ignores the field, the cut or the list filter.
