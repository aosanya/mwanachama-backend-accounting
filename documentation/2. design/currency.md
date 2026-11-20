# Currency on amounts

An amount in this ledger carries a currency. A total never adds two
currencies. A report states the currency it is in, the rule that converted it
and the rates it used. Rates are entered; nothing is fetched.

Every name, date and rate below is invented to show the arithmetic.

## What carries a currency

| Amount | Where the code lives |
| --- | --- |
| Entry | `currency` on the entry. Debit and credit are one amount in one currency, so the ledger still balances, now once for each currency |
| Budget line | `currency` on the budget. For a donor-funded line it is the donor's currency |
| Milestone, certificate | `currency` on each. A certificate and the entries it posts take the milestone's currency |
| Quote | `currency` on the quote |
| Bill | A bill is an entry, so its code is the entry's. `Bill.currency` reads it; `Settled` and `Outstanding` are in it |
| Payment | A payment is an entry that settles another. It must be in the currency of what it settles, and a reversal must be in the currency of the entry it reverses. Both are refused with `ErrMixedCurrency` otherwise |

A row with no code reads as the deployment's default currency. The default is
a constructor option, `WithDefaultCurrency`; without it the default is `XXX`,
the ISO 4217 code for no currency. A code that is not on the list is
`ErrInvalid`.

An amount stays an integer count of the smallest unit of its own currency. The
currency list holds each code and its decimals (ISO 4217): EUR and KES have
two, JPY none.

## Totals never add two currencies

| Read | What it does with two currencies |
| --- | --- |
| Account balance | Refused with `ErrMixedCurrency`; `account_balances_by_currency` gives one balance for each |
| Closing an account | Refused unless the balance is zero in every currency, so +100 EUR and -100 KES is not zero |
| Budget line and variance | `spent` and `actual` count only entries in the line's own currency; `other_spent` lists the rest, one amount for each currency |
| Financial report | Refused with `ErrMixedCurrency` when its budget lines are in more than one currency; the converted report is the way to read it |
| Closing balances in a report | One row for each account type and currency |
| Totals by dimension | One row for each value and currency. A row in the default currency leaves `currency` out. Entries without the key are in `without` for the default currency and `without_other` for the rest |

## Exchange rates

One row for each rate, append-only:

| Field | Meaning |
| --- | --- |
| `base`, `counter` | 1 base buys `rate / 10^8` of the counter. 140.00 KES per EUR is base EUR, counter KES, rate 14,000,000,000 |
| `applies_on` | The date, `2006-01-02`. A monthly rate applies to the whole month of this date |
| `kind` | `daily`, `monthly` or `agreed` |
| `source` | Where the rate comes from, as the donor's rule names it. Required |
| `replaces_id` | A correction is a new row naming the one it replaces, with the same pair, kind and date. A row can be replaced once |

A pair is held in one direction: entering EUR per KES when KES per EUR is held
is refused. The role is optional in a domain spec, like `upward_report`; a
spec that does not declare `rate` answers `ErrInvalid` to the rate calls and
cannot convert.

## Four rules

The converted report (`get_converted_report`) takes `reporting_currency` and
one rule.

| Rule | Rate used | Rounding |
| --- | --- | --- |
| `posting_date` | The daily or agreed rate on the day the entry happened | Once for each entry |
| `posting_month` | The monthly rate of the month the entry happened in | Once for each entry |
| `period_average` | The mean of the daily rates inside the period, applied to the budget line's total in each currency | Once for each currency total of a line |
| `report_date` | The daily or agreed rate on the last day of the period, applied to each currency total | Once for each currency total of a line |

Rounding is half away from zero, to the smallest unit of the reporting
currency, in big-integer arithmetic. A budget line in another currency than
the reporting one is converted at the rate of the last day of the period.

A rate that does not exist is never guessed. The figure leaves that amount
out, the line and the report list the missing pair and date, and `complete`
is false. The amounts as entered show beside the converted ones either way.
The report prints every rate it used with its source and, for the average
rule, the mean and the period.

## Exchange difference

A payment in a currency other than the reporting one, made inside the period,
shows its value in the reporting currency at the rate of the day the bill was
raised, its value at the rate of the day it was paid, and the difference, as
its own line, never inside the spending. A negative difference is a `loss`.
When either rate is missing the line is left out and the missing rate is
listed.

## Upward report

`reporting_currency` and `rule` are asked together or not at all. With them
the report is the converted report, and the record keeps the reporting
currency, the rule, the rates used and the amounts as entered, so a rate
corrected later never changes a report sent. A missing rate refuses the send
with `ErrMissingRate`. Without them the report is as before.

## Worked example

Budget line EUR 25,000.00 for February and March. Spending: KES 1,400,000.00
on 10 February, KES 1,278,000.00 on 12 March, EUR 3,000.00 on 20 March. Daily
rates, KES per EUR: 140.00 on 10 February, 142.00 on 12 March. Monthly: 139.00
for February, 143.00 for March.

| Rule | Spent (EUR) |
| --- | --- |
| `posting_date` | 22,000.00 |
| `posting_month` | 22,009.00 |
| `period_average` (mean 141.00) | 21,992.91 |

A bill of KES 1,420,000.00 raised on 31 March at 142.00 and paid on 20 April
at 144.00 is EUR 10,000.00 and then EUR 9,861.11: an exchange loss of
EUR 138.89. The tests in `currency_test.go` and `currency_internal_test.go`
reproduce these figures.

## Not built

- A payment in another currency than its bill. The first build settles a bill
  only in its own currency, so the exchange difference comes from the rate
  moving between billing and payment, not from paying in another currency.
- A currency on a consultant's day rate (`day_allowance.rate`).
- Currency on a currency answer in forms and agency records, and the approval
  limit in actor. They are other repositories.
- A feed of rates, revaluation entries, several reporting currencies in one
  report, and conversion through a third currency.
