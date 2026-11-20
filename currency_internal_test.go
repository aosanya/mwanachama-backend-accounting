package accounting

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/models"
)

func currencyOf(t *testing.T, code string) models.Currency {
	t.Helper()
	c, ok := models.CurrencyOf(code)
	if !ok {
		t.Fatalf("no currency %s", code)
	}
	return c
}

func TestConvertingKeepsTheSmallestUnitOfEachCurrency(t *testing.T) {
	eurPerKes := ExchangeRate{Base: "EUR", Counter: "KES", Rate: 140 * models.RateScale}
	cases := []struct {
		name     string
		amount   int64
		from, to string
		rate     ExchangeRate
		want     int64
	}{
		{"counter to base", 140_000_000, "KES", "EUR", eurPerKes, 1_000_000},
		{"base to counter", 1_000_000, "EUR", "KES", eurPerKes, 140_000_000},
		{"rounds below a half down", 127_800_000, "KES", "EUR", ExchangeRate{Base: "EUR", Counter: "KES", Rate: 143 * models.RateScale}, 893_706},
		{"rounds a half up", 1, "EUR", "JPY", ExchangeRate{Base: "EUR", Counter: "JPY", Rate: 50 * models.RateScale}, 1},
		{"rounds a negative half away from zero", -1, "EUR", "JPY", ExchangeRate{Base: "EUR", Counter: "JPY", Rate: 50 * models.RateScale}, -1},
		{"to a currency with no decimals", 10_000, "EUR", "JPY", ExchangeRate{Base: "EUR", Counter: "JPY", Rate: 160 * models.RateScale}, 16_000},
		{"from a currency with no decimals", 16_000, "JPY", "EUR", ExchangeRate{Base: "EUR", Counter: "JPY", Rate: 160 * models.RateScale}, 10_000},
		{"a large amount does not overflow", 900_000_000_000_000, "KES", "EUR", eurPerKes, 6_428_571_428_571},
	}
	for _, c := range cases {
		if got := convertMinor(c.amount, currencyOf(t, c.from), currencyOf(t, c.to), c.rate); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestTheAverageRateIsTheMeanOfTheDailyRowsInsideThePeriodOnly(t *testing.T) {
	book := rateBook{
		{Base: "EUR", Counter: "KES", Rate: 100 * models.RateScale, AppliesOn: "2027-01-31", Kind: models.RateDaily},
		{Base: "EUR", Counter: "KES", Rate: 140 * models.RateScale, AppliesOn: "2027-02-10", Kind: models.RateDaily},
		{Base: "EUR", Counter: "KES", Rate: 142 * models.RateScale, AppliesOn: "2027-03-31", Kind: models.RateDaily},
		{Base: "EUR", Counter: "KES", Rate: 999 * models.RateScale, AppliesOn: "2027-02-15", Kind: models.RateMonthly},
		{Base: "EUR", Counter: "KES", Rate: 500 * models.RateScale, AppliesOn: "2027-04-01", Kind: models.RateDaily},
	}
	from := parseDay(t, "2027-02-01")
	to := parseDay(t, "2027-04-01")
	got, ok := book.averageOver("KES", "EUR", from, to)
	if !ok || got.Rate != 141*models.RateScale {
		t.Fatalf("got %+v %v", got, ok)
	}
	if _, ok := book.averageOver("USD", "EUR", from, to); ok {
		t.Fatal("an average for a pair with no rows")
	}
}

func TestTheDailyLookupNeverReadsAMonthlyRowAndTheMonthlyLookupNeverReadsADailyOne(t *testing.T) {
	book := rateBook{
		{Base: "EUR", Counter: "KES", Rate: 139 * models.RateScale, AppliesOn: "2027-02-01", Kind: models.RateMonthly},
		{Base: "EUR", Counter: "KES", Rate: 140 * models.RateScale, AppliesOn: "2027-02-10", Kind: models.RateDaily},
	}
	if _, ok := book.onDay("KES", "EUR", "2027-02-01"); ok {
		t.Fatal("a monthly row answered a daily lookup")
	}
	if r, ok := book.inMonth("KES", "EUR", "2027-02-27"); !ok || r.Rate != 139*models.RateScale {
		t.Fatalf("monthly lookup %+v %v", r, ok)
	}
	if _, ok := book.inMonth("KES", "EUR", "2027-03-01"); ok {
		t.Fatal("a rate of February answered for March")
	}
}

func openInternalLedger(t *testing.T) (*ledgerStore, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s, err := SpecFor("currency")
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if err := Provision(db, s); err != nil {
		t.Fatalf("provision: %v", err)
	}
	repo, err := NewLedgerRepository(db, s, WithDefaultCurrency("EUR"))
	if err != nil {
		t.Fatalf("repository: %v", err)
	}
	return repo.(*ledgerStore), context.Background()
}

func TestAKesBillPaidAtAMovedRateShowsEUR13889AsItsOwnExchangeLoss(t *testing.T) {
	m, ctx := openInternalLedger(t)
	open := func(category string, kind AccountType) Account {
		a, err := m.OpenAccount(ctx, Account{Category: AccountCategory(category), HolderID: "project", Type: kind})
		if err != nil {
			t.Fatalf("open %s: %v", category, err)
		}
		return a
	}
	income, owed, cash, workshops := open("fees", AccountTypeIncome), open("receivable", AccountTypeAsset), open("cash", AccountTypeAsset), open("workshops", AccountTypeExpense)
	for _, r := range []RateInput{
		{Base: "EUR", Counter: "KES", Rate: 142 * models.RateScale, AppliesOn: "2027-03-31", Kind: "daily", Source: "test table", RecordedBy: "finance"},
		{Base: "EUR", Counter: "KES", Rate: 144 * models.RateScale, AppliesOn: "2027-04-20", Kind: "daily", Source: "test table", RecordedBy: "finance"},
	} {
		if _, err := m.CreateRate(ctx, r); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	m.now = func() string { return "2027-03-31T10:00:00Z" }
	bill, err := m.CreateBill(ctx, BillInput{
		IncomeAccountID: income.ID, ReceivableAccountID: owed.ID, Amount: 142_000_000, Currency: "KES",
		DueAt: "2027-05-15T00:00:00Z", BilledBy: "finance",
	})
	if err != nil {
		t.Fatalf("bill: %v", err)
	}
	m.now = func() string { return "2027-04-21T10:00:00Z" }
	payment, err := m.MarkBillPaid(ctx, MarkPaidInput{BillID: bill.ID, CashAccountID: cash.ID, PaidOn: "2027-04-20T08:00:00Z", PaidBy: "finance"})
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	if _, err := m.SetBudget(ctx, Budget{AccountID: workshops.ID, PeriodStart: "2027-04-01T00:00:00Z", PeriodEnd: "2027-05-01T00:00:00Z", Amount: 100, Currency: "EUR"}); err != nil {
		t.Fatalf("budget: %v", err)
	}
	report, err := m.GetConvertedReport(ctx, ConvertedReportInput{
		PeriodStart: "2027-04-01T00:00:00Z", PeriodEnd: "2027-05-01T00:00:00Z", ReportingCurrency: "EUR", Rule: RuleRateAtPostingDate,
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	want := ExchangeDifference{
		BillID: bill.ID, PaymentID: payment.ID, Currency: "KES", Paid: 142_000_000,
		AtBilling: 1_000_000, AtPayment: 986_111, Difference: -13_889, Outcome: "loss",
	}
	if len(report.ExchangeDifferences) != 1 || report.ExchangeDifferences[0] != want {
		t.Fatalf("differences %+v, want %+v", report.ExchangeDifferences, want)
	}
	if report.Spent != 0 {
		t.Fatalf("the exchange difference was folded into spending: %d", report.Spent)
	}
}

func TestAnExchangeDifferenceWithoutBothRatesIsNotInventedAndTheReportSaysWhichRateIsMissing(t *testing.T) {
	m, ctx := openInternalLedger(t)
	open := func(category string, kind AccountType) Account {
		a, _ := m.OpenAccount(ctx, Account{Category: AccountCategory(category), HolderID: "project", Type: kind})
		return a
	}
	income, owed, cash, workshops := open("fees", AccountTypeIncome), open("receivable", AccountTypeAsset), open("cash", AccountTypeAsset), open("workshops", AccountTypeExpense)
	if _, err := m.CreateRate(ctx, RateInput{Base: "EUR", Counter: "KES", Rate: 142 * models.RateScale, AppliesOn: "2027-03-31", Kind: "daily", Source: "s", RecordedBy: "f"}); err != nil {
		t.Fatalf("rate: %v", err)
	}
	m.now = func() string { return "2027-03-31T10:00:00Z" }
	bill, _ := m.CreateBill(ctx, BillInput{IncomeAccountID: income.ID, ReceivableAccountID: owed.ID, Amount: 142_000_000, Currency: "KES", DueAt: "2027-05-15T00:00:00Z", BilledBy: "f"})
	m.now = func() string { return "2027-04-21T10:00:00Z" }
	if _, err := m.MarkBillPaid(ctx, MarkPaidInput{BillID: bill.ID, CashAccountID: cash.ID, PaidOn: "2027-04-20T08:00:00Z", PaidBy: "f"}); err != nil {
		t.Fatalf("pay: %v", err)
	}
	if _, err := m.SetBudget(ctx, Budget{AccountID: workshops.ID, PeriodStart: "2027-04-01T00:00:00Z", PeriodEnd: "2027-05-01T00:00:00Z", Amount: 100, Currency: "EUR"}); err != nil {
		t.Fatalf("budget: %v", err)
	}
	report, err := m.GetConvertedReport(ctx, ConvertedReportInput{
		PeriodStart: "2027-04-01T00:00:00Z", PeriodEnd: "2027-05-01T00:00:00Z", ReportingCurrency: "EUR", Rule: RuleRateAtPostingDate,
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if len(report.ExchangeDifferences) != 0 || report.Complete || len(report.Missing) != 1 || report.Missing[0].On != "2027-04-20" {
		t.Fatalf("report %+v", report)
	}
}

func parseDay(t *testing.T, day string) time.Time {
	t.Helper()
	at, err := time.Parse(dateLayout, day)
	if err != nil {
		t.Fatalf("day %s: %v", day, err)
	}
	return at
}
