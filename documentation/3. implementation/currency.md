# Currency on amounts: what was built

Design: [currency.md](../2.%20design/currency.md).

Status: built and tested on the in-memory SQLite side, not reviewed by the owner.

## Files

| File | What it holds |
| --- | --- |
| `models/currency.go` | The currency list, the rate and report types, the four rules |
| `currency.go` | `WithDefaultCurrency`, the default, per-currency balances, `ErrMixedCurrency`, `ErrMissingRate` |
| `rate_store.go` | `CreateRate`, `ListRates`, the optional `rate` role |
| `conversion.go` | The rate book, the lookups, the average, `convertMinor` |
| `converted_report.go` | `GetConvertedReport`, exchange differences, the converted upward report |
| `accounting.blueprint.json` | `currency` on entry, budget, milestone, certificate and quote; four fields on `upward_report`; the `rate` role |
| `accounting.operations.json` | `create_rate`, `list_rates`, `get_converted_report`, `account_balances_by_currency`; the errors `ErrMixedCurrency` and `ErrMissingRate`, both 409 |
| `mwanachama.accounting.json` | The `rate` object, so the shipped domain keeps rates |

## For a consumer

- `NewLedgerRepository` takes options after the spec; none is required.
- An instance spec that does not declare the `rate` role keeps working and
  refuses the rate calls.
- The four new operations sort between existing names, so a mount that pairs
  routes with `Shape()` by index reads the table again after the tag.
- `Balance` and `DimensionBalance` answer `ErrMixedCurrency` for an account
  that holds two currencies; callers use `BalancesByCurrency`.
- `LedgerRepository` gained `RateRepository`, `ConvertedReportRepository` and
  `BalancesByCurrency`.

## Tests

`currency_test.go`, `currency_internal_test.go` and `routes/currency_routes_test.go`.
The internal tests set the ledger's clock to date a bill and its payment.
Each behaviour was shown to fail when the code behind it was changed.

## Left

Forms, agency records and the actor approval limit, in their own repositories;
a currency on a day rate; a payment in another currency than its bill.
