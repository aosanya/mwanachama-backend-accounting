# Upward report: recipient, recipient row and reference

An upward report can name who it was sent to. Three optional fields on the `upward_report` role, set at send time and kept as sent.

Status: built and tested on the in-memory SQLite side, not reviewed by the owner.

## The fields

| Field | Holds | Checked |
|---|---|---|
| `recipient` | The recipient's name as the sender wrote it | Trimmed; free text |
| `recipient_record_row_id` | The recipient's row in a record the caller keeps | Trimmed; a uuid, else `ErrInvalid` and nothing is stored |
| `reference` | The recipient's own number for what the report concerns, such as a contract or grant number | Trimmed; free text |

All three are immutable, like the rest of the record. They follow how a bill names its payer (`payer`, `payer_actor_id`, `payer_record_row_id`) and do not replace anything: a report sent without them reads as it did before and omits them from its JSON.

## The list

`list_upward_reports` takes `recipient_record_row_id` and keeps only the reports sent to that row. Order stays newest first and the limit still applies. A blank value lists every report. The blueprint indexes the column.

## What this does not do

The figures still cover every budget line of the period, not one project, and amounts carry no currency. Two sends of one period to two recipients are two records, each with its own recipient.

## Tests

`upward_report_test.go` holds the store cases and `routes/reporting_routes_test.go` the HTTP case. Four store tests and the HTTP test fail when the store ignores the fields; the other two guard the old behaviour.
