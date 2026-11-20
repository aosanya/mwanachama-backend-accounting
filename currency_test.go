package accounting_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	febStart  = "2027-02-01T00:00:00Z"
	aprStart  = "2027-04-01T00:00:00Z"
	scale     = int64(100_000_000)
	kesPerEUR = 140 * scale
)

type currencyLedger struct {
	repo      accounting.LedgerRepository
	workshops accounting.Account
	cash      accounting.Account
	income    accounting.Account
	owed      accounting.Account
}

func newCurrencyLedger(t *testing.T) currencyLedger {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s, err := accounting.SpecFor("currency")
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if err := accounting.Provision(db, s); err != nil {
		t.Fatalf("provision: %v", err)
	}
	repo, err := accounting.NewLedgerRepository(db, s, accounting.WithDefaultCurrency("EUR"))
	if err != nil {
		t.Fatalf("repository: %v", err)
	}
	open := func(code, category string, kind accounting.AccountType) accounting.Account {
		a, err := repo.OpenAccount(ctxFor(), accounting.Account{Code: code, Category: accounting.AccountCategory(category), HolderID: "project-ke-01", Type: kind})
		if err != nil {
			t.Fatalf("open account %s: %v", code, err)
		}
		return a
	}
	return currencyLedger{
		repo:      repo,
		workshops: open("5100", "training_workshops", accounting.AccountTypeExpense),
		cash:      open("1000", "cash", accounting.AccountTypeAsset),
		income:    open("4000", "fees", accounting.AccountTypeIncome),
		owed:      open("1200", "receivable", accounting.AccountTypeAsset),
	}
}

func (l currencyLedger) spend(t *testing.T, amount int64, currency, on, document string) accounting.Entry {
	t.Helper()
	e, err := l.repo.Post(ctxFor(), accounting.Entry{
		DebitAccountID: l.workshops.ID, CreditAccountID: l.cash.ID, Amount: amount, Currency: currency,
		OccurredAt: on, DocumentKind: "expense", DocumentID: document, ActorID: "finance",
	})
	if err != nil {
		t.Fatalf("post %s %d: %v", currency, amount, err)
	}
	return e
}

func (l currencyLedger) rate(t *testing.T, counter string, value int64, on, kind string) accounting.ExchangeRate {
	t.Helper()
	r, err := l.repo.CreateRate(ctxFor(), accounting.RateInput{
		Base: "EUR", Counter: counter, Rate: value, AppliesOn: on, Kind: kind, Source: "test table", RecordedBy: "finance",
	})
	if err != nil {
		t.Fatalf("rate %s %s: %v", counter, on, err)
	}
	return r
}

func (l currencyLedger) budget(t *testing.T, start, end string) {
	t.Helper()
	if _, err := l.repo.SetBudget(ctxFor(), accounting.Budget{
		AccountID: l.workshops.ID, PeriodStart: start, PeriodEnd: end, Name: "Training workshops",
		Department: "Kenya", Amount: 2_500_000, Currency: "EUR",
	}); err != nil {
		t.Fatalf("budget: %v", err)
	}
}

func (l currencyLedger) workedExample(t *testing.T) {
	t.Helper()
	l.spend(t, 140_000_000, "KES", "2027-02-10T09:00:00Z", "venue")
	l.spend(t, 127_800_000, "KES", "2027-03-12T09:00:00Z", "trainers")
	l.spend(t, 300_000, "EUR", "2027-03-20T09:00:00Z", "expert")
	l.rate(t, "KES", kesPerEUR, "2027-02-10", "daily")
	l.rate(t, "KES", 142*scale, "2027-03-12", "daily")
	l.rate(t, "KES", 139*scale, "2027-02-01", "monthly")
	l.rate(t, "KES", 143*scale, "2027-03-01", "monthly")
	l.budget(t, febStart, aprStart)
}

func (l currencyLedger) convertedReport(t *testing.T, rule accounting.ConversionRule) accounting.ConvertedReport {
	t.Helper()
	r, err := l.repo.GetConvertedReport(ctxFor(), accounting.ConvertedReportInput{
		PeriodStart: febStart, PeriodEnd: aprStart, ReportingCurrency: "EUR", Rule: rule,
	})
	if err != nil {
		t.Fatalf("converted report %s: %v", rule, err)
	}
	return r
}

func TestAnEntryCarriesItsCurrencyAndAnOldRowReadsAsTheDefault(t *testing.T) {
	l := newCurrencyLedger(t)
	l.spend(t, 5000, "KES", "2027-02-10T09:00:00Z", "venue")
	l.spend(t, 700, "", "2027-02-11T09:00:00Z", "unlabelled")
	entries, err := l.repo.ListEntries(ctxFor(), l.workshops.ID, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.DocumentID] = e.Currency
	}
	want := map[string]string{"venue": "KES", "unlabelled": "EUR"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("currencies %v, want %v", got, want)
	}
}

func TestAnEntryInAnUnknownCurrencyIsRefused(t *testing.T) {
	l := newCurrencyLedger(t)
	_, err := l.repo.Post(ctxFor(), accounting.Entry{
		DebitAccountID: l.workshops.ID, CreditAccountID: l.cash.ID, Amount: 5, Currency: "KSH",
		DocumentKind: "expense", DocumentID: "x", ActorID: "finance",
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestABalanceIsNeverTheSumOfTwoCurrencies(t *testing.T) {
	l := newCurrencyLedger(t)
	l.spend(t, 5000, "KES", "2027-02-10T09:00:00Z", "venue")
	l.spend(t, 300, "EUR", "2027-02-11T09:00:00Z", "expert")
	if _, err := l.repo.Balance(ctxFor(), l.workshops.ID); !errors.Is(err, accounting.ErrMixedCurrency) {
		t.Fatalf("got %v, want ErrMixedCurrency", err)
	}
	got, err := l.repo.BalancesByCurrency(ctxFor(), l.workshops.ID)
	if err != nil {
		t.Fatalf("by currency: %v", err)
	}
	want := []accounting.CurrencyAmount{{Currency: "EUR", Amount: 300}, {Currency: "KES", Amount: 5000}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("balances %v, want %v", got, want)
	}
}

func TestAnAccountThatNetsToZeroOnlyAcrossCurrenciesCannotBeClosed(t *testing.T) {
	l := newCurrencyLedger(t)
	l.spend(t, 500, "EUR", "2027-02-10T09:00:00Z", "a")
	if _, err := l.repo.Post(ctxFor(), accounting.Entry{
		DebitAccountID: l.cash.ID, CreditAccountID: l.workshops.ID, Amount: 500, Currency: "KES",
		DocumentKind: "refund", DocumentID: "b", ActorID: "finance",
	}); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := l.repo.CloseAccount(ctxFor(), l.workshops.ID); !errors.Is(err, accounting.ErrNonZeroBalance) {
		t.Fatalf("got %v, want ErrNonZeroBalance", err)
	}
}

func TestAPaymentSettlesABillOnlyInTheBillsCurrency(t *testing.T) {
	l := newCurrencyLedger(t)
	bill, err := l.repo.CreateBill(ctxFor(), accounting.BillInput{
		IncomeAccountID: l.income.ID, ReceivableAccountID: l.owed.ID, Amount: 142_000_000, Currency: "KES",
		DueAt: "2027-04-30T00:00:00Z", BilledBy: "finance",
	})
	if err != nil {
		t.Fatalf("bill: %v", err)
	}
	if bill.Currency != "KES" || bill.Outstanding != 142_000_000 {
		t.Fatalf("bill %+v", bill)
	}
	_, err = l.repo.Post(ctxFor(), accounting.Entry{
		DebitAccountID: l.cash.ID, CreditAccountID: l.owed.ID, Amount: 100, Currency: "EUR",
		DocumentKind: "payment", DocumentID: "p", ActorID: "finance", SettlesEntryID: bill.ID,
	})
	if !errors.Is(err, accounting.ErrMixedCurrency) {
		t.Fatalf("got %v, want ErrMixedCurrency", err)
	}
	payment, err := l.repo.MarkBillPaid(ctxFor(), accounting.MarkPaidInput{BillID: bill.ID, CashAccountID: l.cash.ID, Amount: 1000, PaidBy: "finance"})
	if err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if payment.Currency != "KES" {
		t.Fatalf("payment currency %q", payment.Currency)
	}
	settled, _ := l.repo.GetBill(ctxFor(), bill.ID)
	if settled.Settled != 1000 || settled.Currency != "KES" {
		t.Fatalf("bill after payment %+v", settled)
	}
}

func TestABudgetLineCountsOnlySpendingInItsOwnCurrencyAndListsTheRest(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	lines, err := l.repo.ListBudgetLines(ctxFor(), accounting.BudgetLineFilter{AccountID: l.workshops.ID})
	if err != nil || len(lines) != 1 {
		t.Fatalf("lines %v %v", lines, err)
	}
	line := lines[0]
	if line.Currency != "EUR" || line.Planned != 2_500_000 || line.Spent != 300_000 {
		t.Fatalf("line %+v", line)
	}
	if !reflect.DeepEqual(line.OtherSpent, []accounting.CurrencyAmount{{Currency: "KES", Amount: 267_800_000}}) {
		t.Fatalf("other spent %v", line.OtherSpent)
	}
	v, err := l.repo.Variance(ctxFor(), l.workshops.ID, febStart, aprStart)
	if err != nil || v.Currency != "EUR" || v.Actual != 300_000 {
		t.Fatalf("variance %+v %v", v, err)
	}
}

func TestTheFinancialReportRefusesBudgetLinesInTwoCurrencies(t *testing.T) {
	l := newCurrencyLedger(t)
	l.budget(t, febStart, aprStart)
	other, err := l.repo.OpenAccount(ctxFor(), accounting.Account{Category: "travel", HolderID: "project-ke-01", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := l.repo.SetBudget(ctxFor(), accounting.Budget{AccountID: other.ID, PeriodStart: febStart, PeriodEnd: aprStart, Amount: 1000, Currency: "KES"}); err != nil {
		t.Fatalf("budget: %v", err)
	}
	if _, err := l.repo.GetReport(ctxFor(), febStart, aprStart); !errors.Is(err, accounting.ErrMixedCurrency) {
		t.Fatalf("got %v, want ErrMixedCurrency", err)
	}
}

func TestARateIsEnteredListedAndCorrectedByANewRow(t *testing.T) {
	l := newCurrencyLedger(t)
	first := l.rate(t, "KES", kesPerEUR, "2027-02-10", "daily")
	fixed, err := l.repo.CreateRate(ctxFor(), accounting.RateInput{
		Base: "EUR", Counter: "KES", Rate: 141 * scale, AppliesOn: "2027-02-10", Kind: "daily",
		Source: "corrected table", ReplacesID: first.ID, RecordedBy: "finance",
	})
	if err != nil {
		t.Fatalf("correction: %v", err)
	}
	listed, err := l.repo.ListRates(ctxFor(), accounting.RateFilter{})
	if err != nil || len(listed) != 1 || listed[0].ID != fixed.ID || listed[0].Rate != 141*scale {
		t.Fatalf("listed %+v %v", listed, err)
	}
	_, err = l.repo.CreateRate(ctxFor(), accounting.RateInput{
		Base: "EUR", Counter: "KES", Rate: 142 * scale, AppliesOn: "2027-02-10", Kind: "daily",
		Source: "again", ReplacesID: first.ID, RecordedBy: "finance",
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("second correction of one row: got %v, want ErrInvalid", err)
	}
}

func TestARateIsRefusedWhenItsPairIsHeldTheOtherWayRoundOrItIsMalformed(t *testing.T) {
	l := newCurrencyLedger(t)
	l.rate(t, "KES", kesPerEUR, "2027-02-10", "daily")
	bad := []accounting.RateInput{
		{Base: "KES", Counter: "EUR", Rate: 714_286, AppliesOn: "2027-02-11", Kind: "daily", Source: "s", RecordedBy: "f"},
		{Base: "EUR", Counter: "EUR", Rate: scale, AppliesOn: "2027-02-11", Kind: "daily", Source: "s", RecordedBy: "f"},
		{Base: "EUR", Counter: "KES", Rate: 0, AppliesOn: "2027-02-11", Kind: "daily", Source: "s", RecordedBy: "f"},
		{Base: "EUR", Counter: "KES", Rate: scale, AppliesOn: "11/02/2027", Kind: "daily", Source: "s", RecordedBy: "f"},
		{Base: "EUR", Counter: "KES", Rate: scale, AppliesOn: "2027-02-11", Kind: "weekly", Source: "s", RecordedBy: "f"},
		{Base: "EUR", Counter: "KES", Rate: scale, AppliesOn: "2027-02-11", Kind: "daily", Source: " ", RecordedBy: "f"},
	}
	for _, in := range bad {
		if _, err := l.repo.CreateRate(ctxFor(), in); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%+v: got %v, want ErrInvalid", in, err)
		}
	}
}

func TestThePostingDateRuleSpendsEUR22000OfTheWorkedExample(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	r := l.convertedReport(t, accounting.RuleRateAtPostingDate)
	if r.Spent != 2_200_000 || r.Planned != 2_500_000 || r.Difference != 300_000 || !r.Complete {
		t.Fatalf("report %+v", r)
	}
	if got := r.Lines[0].Spent; got != 2_200_000 {
		t.Fatalf("line spent %d", got)
	}
}

func TestThePostingMonthRuleSpendsEUR22009OfTheWorkedExample(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	r := l.convertedReport(t, accounting.RuleRateOfPostingMonth)
	if r.Spent != 2_200_900 || r.Difference != 299_100 || !r.Complete {
		t.Fatalf("report %+v", r)
	}
}

func TestThePeriodAverageRuleSpendsEUR2199291OfTheWorkedExample(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	r := l.convertedReport(t, accounting.RuleAverageOverPeriod)
	if r.Spent != 2_199_291 || r.Difference != 300_709 || !r.Complete {
		t.Fatalf("report %+v", r)
	}
	if len(r.Rates) != 1 || r.Rates[0].Rate != 141*scale || r.Rates[0].Kind != "average" {
		t.Fatalf("rates %+v", r.Rates)
	}
}

func TestTheReportDateRuleConvertsEachCurrencyTotalAtTheRateOfTheLastDay(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	l.rate(t, "KES", 143*scale, "2027-03-31", "daily")
	r := l.convertedReport(t, accounting.RuleRateAtReportDate)
	if r.Spent != 2_172_727 || !r.Complete {
		t.Fatalf("report %+v", r)
	}
}

func TestTheReportShowsTheOriginalAmountsTheRuleAndTheRatesUsed(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	r := l.convertedReport(t, accounting.RuleRateAtPostingDate)
	wantOriginal := []accounting.CurrencyAmount{{Currency: "EUR", Amount: 300_000}, {Currency: "KES", Amount: 267_800_000}}
	if !reflect.DeepEqual(r.Original, wantOriginal) {
		t.Fatalf("original %v", r.Original)
	}
	if r.Rule != accounting.RuleRateAtPostingDate || r.ReportingCurrency != "EUR" || len(r.Rates) != 2 {
		t.Fatalf("report %+v", r)
	}
	if r.Rates[0].On != "2027-02-10" || r.Rates[0].Rate != kesPerEUR || r.Rates[0].Source != "test table" {
		t.Fatalf("first rate %+v", r.Rates[0])
	}
}

func TestAMissingRateIsShownAndNeverGuessed(t *testing.T) {
	l := newCurrencyLedger(t)
	l.spend(t, 140_000_000, "KES", "2027-02-10T09:00:00Z", "venue")
	l.spend(t, 127_800_000, "KES", "2027-03-12T09:00:00Z", "trainers")
	l.spend(t, 300_000, "EUR", "2027-03-20T09:00:00Z", "expert")
	l.rate(t, "KES", kesPerEUR, "2027-02-10", "daily")
	l.budget(t, febStart, aprStart)
	r := l.convertedReport(t, accounting.RuleRateAtPostingDate)
	if r.Complete {
		t.Fatalf("report claims to be complete: %+v", r)
	}
	wantMissing := []accounting.MissingRate{{From: "KES", To: "EUR", On: "2027-03-12"}}
	if !reflect.DeepEqual(r.Missing, wantMissing) || !reflect.DeepEqual(r.Lines[0].Missing, wantMissing) {
		t.Fatalf("missing %v, line %v", r.Missing, r.Lines[0].Missing)
	}
	if r.Spent != 1_000_000+300_000 {
		t.Fatalf("spent %d counts only what could be converted", r.Spent)
	}
	if len(r.Original) != 2 || r.Original[1].Amount != 267_800_000 {
		t.Fatalf("the amounts as entered still show: %v", r.Original)
	}
}

func TestAConvertedReportNeedsAKnownCurrencyAndRule(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	for _, in := range []accounting.ConvertedReportInput{
		{PeriodStart: febStart, PeriodEnd: aprStart, ReportingCurrency: "EURO", Rule: accounting.RuleRateAtPostingDate},
		{PeriodStart: febStart, PeriodEnd: aprStart, ReportingCurrency: "EUR", Rule: "whenever"},
	} {
		if _, err := l.repo.GetConvertedReport(ctxFor(), in); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%+v: got %v, want ErrInvalid", in, err)
		}
	}
	_, err := l.repo.GetConvertedReport(ctxFor(), accounting.ConvertedReportInput{
		PeriodStart: "2030-01-01T00:00:00Z", PeriodEnd: "2030-02-01T00:00:00Z", ReportingCurrency: "EUR", Rule: accounting.RuleRateAtPostingDate,
	})
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestASentUpwardReportNamesItsCurrencyRuleAndRatesAndKeepsThemWhenARateIsCorrected(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	sent, err := l.repo.SendUpwardReport(ctxFor(), accounting.UpwardReportInput{
		PeriodStart: febStart, PeriodEnd: aprStart, Kind: "budget_and_actuals", SentBy: "finance",
		ReportingCurrency: "EUR", Rule: accounting.RuleRateOfPostingMonth,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if sent.ReportingCurrency != "EUR" || sent.Rule != accounting.RuleRateOfPostingMonth || sent.Spent != 2_200_900 || len(sent.Rates) != 2 || len(sent.Original) != 2 {
		t.Fatalf("sent %+v", sent)
	}
	rates, _ := l.repo.ListRates(ctxFor(), accounting.RateFilter{Kind: "monthly"})
	if _, err := l.repo.CreateRate(ctxFor(), accounting.RateInput{
		Base: "EUR", Counter: "KES", Rate: 150 * scale, AppliesOn: rates[0].AppliesOn, Kind: "monthly",
		Source: "corrected", ReplacesID: rates[0].ID, RecordedBy: "finance",
	}); err != nil {
		t.Fatalf("correct: %v", err)
	}
	listed, err := l.repo.ListUpwardReports(ctxFor(), accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed %v %v", listed, err)
	}
	if !reflect.DeepEqual(listed[0].Rates, sent.Rates) || listed[0].Spent != 2_200_900 {
		t.Fatalf("a corrected rate changed a sent report: %+v", listed[0])
	}
	again := l.convertedReport(t, accounting.RuleRateOfPostingMonth)
	if again.Spent == 2_200_900 {
		t.Fatalf("the next report did not use the corrected rate")
	}
}

func TestAnUpwardReportWithAMissingRateIsRefused(t *testing.T) {
	l := newCurrencyLedger(t)
	l.spend(t, 140_000_000, "KES", "2027-02-10T09:00:00Z", "venue")
	l.budget(t, febStart, aprStart)
	_, err := l.repo.SendUpwardReport(ctxFor(), accounting.UpwardReportInput{
		PeriodStart: febStart, PeriodEnd: aprStart, Kind: "budget_and_actuals", SentBy: "finance",
		ReportingCurrency: "EUR", Rule: accounting.RuleRateAtPostingDate,
	})
	if !errors.Is(err, accounting.ErrMissingRate) {
		t.Fatalf("got %v, want ErrMissingRate", err)
	}
}

func TestAnUpwardReportNamesBothTheCurrencyAndTheRuleOrNeither(t *testing.T) {
	l := newCurrencyLedger(t)
	l.workedExample(t)
	_, err := l.repo.SendUpwardReport(ctxFor(), accounting.UpwardReportInput{
		PeriodStart: febStart, PeriodEnd: aprStart, Kind: "budget_and_actuals", SentBy: "finance", ReportingCurrency: "EUR",
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestMilestonesCertificatesAndQuotesCarryTheirCurrency(t *testing.T) {
	l := newCurrencyLedger(t)
	ms, err := l.repo.AddMilestone(ctxFor(), accounting.Milestone{AccountID: l.workshops.ID, ContractID: "c-1", Name: "Inception", Amount: 10_000, Currency: "USD"})
	if err != nil || ms.Currency != "USD" {
		t.Fatalf("milestone %+v %v", ms, err)
	}
	bare, err := l.repo.AddMilestone(ctxFor(), accounting.Milestone{AccountID: l.workshops.ID, ContractID: "c-1", Name: "Report", Amount: 10_000})
	if err != nil || bare.Currency != "EUR" {
		t.Fatalf("milestone without a currency %+v %v", bare, err)
	}
	payee, _ := l.repo.OpenAccount(ctxFor(), accounting.Account{Category: "payable", HolderID: "supplier", Type: accounting.AccountTypeLiability})
	retained, _ := l.repo.OpenAccount(ctxFor(), accounting.Account{Category: "retention", HolderID: "supplier", Type: accounting.AccountTypeLiability})
	cert, err := l.repo.CertifyPayment(ctxFor(), accounting.Certificate{
		MilestoneID: ms.ID, PayeeAccountID: payee.ID, RetentionAccountID: retained.ID, Amount: 10_000, CertifiedBy: "lead",
	})
	if err != nil || cert.Currency != "USD" {
		t.Fatalf("certificate %+v %v", cert, err)
	}
	held, _ := l.repo.BalancesByCurrency(ctxFor(), payee.ID)
	if !reflect.DeepEqual(held, []accounting.CurrencyAmount{{Currency: "USD", Amount: -10_000}}) {
		t.Fatalf("payee balances %v", held)
	}
	if _, err := l.repo.AddMilestone(ctxFor(), accounting.Milestone{AccountID: l.workshops.ID, ContractID: "c-1", Name: "Bad", Amount: 1, Currency: "US"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestTotalsByDimensionKeepEachCurrencyInItsOwnRow(t *testing.T) {
	l := newCurrencyLedger(t)
	post := func(amount int64, currency, project, document string) {
		t.Helper()
		if _, err := l.repo.Post(ctxFor(), accounting.Entry{
			DebitAccountID: l.workshops.ID, CreditAccountID: l.cash.ID, Amount: amount, Currency: currency,
			OccurredAt: "2027-02-10T09:00:00Z", DocumentKind: "expense", DocumentID: document, ActorID: "finance",
			Dimensions: map[string]string{"project": project},
		}); err != nil {
			t.Fatalf("post: %v", err)
		}
	}
	post(1000, "KES", "2027-KE-01", "a")
	post(300, "EUR", "2027-KE-01", "b")
	post(50, "KES", "2027-KE-01", "c")
	if _, err := l.repo.Post(ctxFor(), accounting.Entry{
		DebitAccountID: l.workshops.ID, CreditAccountID: l.cash.ID, Amount: 7, Currency: "KES",
		OccurredAt: "2027-02-10T09:00:00Z", DocumentKind: "expense", DocumentID: "loose", ActorID: "finance",
	}); err != nil {
		t.Fatalf("post: %v", err)
	}
	got, err := l.repo.DimensionTotals(ctxFor(), "project", "expense", "", "")
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	want := []accounting.DimensionTotal{
		{Value: "2027-KE-01", Total: 300, Entries: 1},
		{Value: "2027-KE-01", Currency: "KES", Total: 1050, Entries: 2},
	}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals %+v, want %+v", got.Totals, want)
	}
	if got.Without.Total != 0 || !reflect.DeepEqual(got.WithoutOther, []accounting.DimensionTotal{{Currency: "KES", Total: 7, Entries: 1}}) {
		t.Fatalf("without %+v other %+v", got.Without, got.WithoutOther)
	}
	entries, err := l.repo.DimensionEntries(ctxFor(), "project", "2027-KE-01", "expense", "", "")
	if err != nil || len(entries.Entries) != 3 {
		t.Fatalf("entries %+v %v", entries, err)
	}
	seen := map[string]int{}
	for _, e := range entries.Entries {
		seen[e.Currency]++
	}
	if seen[""] != 1 || seen["KES"] != 2 {
		t.Fatalf("entry currencies %v", seen)
	}
}
