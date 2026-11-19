package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	billDue      = "2026-03-01T00:00:00Z"
	beforeDue    = "2026-02-01T00:00:00Z"
	afterDue     = "2026-03-11T00:00:00Z"
	laterDue     = "2026-06-01T00:00:00Z"
	billedByClrk = "clerk-1"
)

func (f storeFixture) receivable(t *testing.T) accounting.Account {
	t.Helper()
	a, err := f.repo.OpenAccount(f.bg, accounting.Account{Code: "1200", Category: "receivable", HolderID: "hub-1", Type: accounting.AccountTypeAsset})
	if err != nil {
		t.Fatalf("OpenAccount: %v", err)
	}
	return a
}

func (f storeFixture) bill(t *testing.T, receivable accounting.Account, payer string, amount int64, due string) accounting.Bill {
	t.Helper()
	b, err := f.repo.CreateBill(f.bg, accounting.BillInput{
		IncomeAccountID: f.sales.ID, ReceivableAccountID: receivable.ID, Payer: payer,
		Amount: amount, DueAt: due, Narration: "Market stall fee", BilledBy: billedByClrk,
	})
	if err != nil {
		t.Fatalf("CreateBill: %v", err)
	}
	return b
}

func (f storeFixture) pay(t *testing.T, receivable accounting.Account, b accounting.Bill, amount int64, document string) {
	t.Helper()
	f.postAt(t, f.cash, receivable, amount, accounting.Entry{SettlesEntryID: b.ID, DocumentKind: "receipt", DocumentID: document})
}

func TestABillReadsBackOpenWithItsDueDateAndPayer(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	made := f.bill(t, rec, "stall-holder-7", 1200, billDue)

	got, err := f.repo.GetBill(f.bg, made.ID)
	if err != nil {
		t.Fatalf("GetBill: %v", err)
	}
	if got.Amount != 1200 || got.Outstanding != 1200 || got.Settled != 0 || got.DueAt != billDue ||
		got.Payer != "stall-holder-7" || got.IncomeAccountID != f.sales.ID || got.ReceivableAccountID != rec.ID ||
		got.BilledBy != billedByClrk || got.Narration != "Market stall fee" {
		t.Fatalf("bill %+v", got)
	}
}

func TestABillPostsTheReceivableAgainstTheIncomeAccount(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	f.bill(t, rec, "", 700, billDue)

	owed, err := f.repo.Balance(f.bg, rec.ID)
	if err != nil || owed != 700 {
		t.Fatalf("receivable balance %d %v", owed, err)
	}
}

func TestBillsAreOpenBeforeTheirDueDateAndOverdueAfterIt(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	f.bill(t, rec, "a", 500, billDue)

	before, err := f.repo.ListBills(f.bg, accounting.BillFilter{AsOf: beforeDue})
	if err != nil || len(before) != 1 || before[0].Status != "open" || before[0].DaysOverdue != 0 {
		t.Fatalf("before %+v %v", before, err)
	}
	after, err := f.repo.ListBills(f.bg, accounting.BillFilter{AsOf: afterDue})
	if err != nil || len(after) != 1 || after[0].Status != "overdue" || after[0].DaysOverdue != 10 {
		t.Fatalf("after %+v %v", after, err)
	}
}

func TestAPartPaidBillStaysOverdueForTheRestAndAFullyPaidOneIsSettled(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	part := f.bill(t, rec, "a", 500, billDue)
	whole := f.bill(t, rec, "b", 300, billDue)
	f.pay(t, rec, part, 200, "rcpt-1")
	f.pay(t, rec, whole, 300, "rcpt-2")

	got, err := f.repo.ListBills(f.bg, accounting.BillFilter{AsOf: afterDue})
	if err != nil || len(got) != 2 {
		t.Fatalf("bills %+v %v", got, err)
	}
	byID := map[string]accounting.Bill{}
	for _, b := range got {
		byID[b.ID] = b
	}
	if p := byID[part.ID]; p.Status != "overdue" || p.Settled != 200 || p.Outstanding != 300 {
		t.Fatalf("part %+v", p)
	}
	if w := byID[whole.ID]; w.Status != "settled" || w.Settled != 300 || w.Outstanding != 0 {
		t.Fatalf("whole %+v", w)
	}
}

func TestListingBillsNarrowsByStatusPayerAndIncomeAccount(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	f.bill(t, rec, "a", 100, billDue)
	paid := f.bill(t, rec, "b", 200, billDue)
	f.bill(t, rec, "a", 300, laterDue)
	f.pay(t, rec, paid, 200, "rcpt-1")

	overdue, err := f.repo.ListBills(f.bg, accounting.BillFilter{Status: "overdue", AsOf: afterDue})
	if err != nil || len(overdue) != 1 || overdue[0].Amount != 100 {
		t.Fatalf("overdue %+v %v", overdue, err)
	}
	settled, err := f.repo.ListBills(f.bg, accounting.BillFilter{Status: "settled", AsOf: afterDue})
	if err != nil || len(settled) != 1 || settled[0].ID != paid.ID {
		t.Fatalf("settled %+v %v", settled, err)
	}
	byPayer, err := f.repo.ListBills(f.bg, accounting.BillFilter{Payer: "a", AsOf: afterDue})
	if err != nil || len(byPayer) != 2 {
		t.Fatalf("payer %+v %v", byPayer, err)
	}
	other, err := f.repo.OpenAccount(f.bg, accounting.Account{Code: "4100", Category: "permits", HolderID: "hub-1", Type: accounting.AccountTypeIncome})
	if err != nil {
		t.Fatal(err)
	}
	none, err := f.repo.ListBills(f.bg, accounting.BillFilter{IncomeAccountID: other.ID, AsOf: afterDue})
	if err != nil || len(none) != 0 {
		t.Fatalf("other income %+v %v", none, err)
	}
	capped, err := f.repo.ListBills(f.bg, accounting.BillFilter{Limit: 2, AsOf: afterDue})
	if err != nil || len(capped) != 2 || capped[0].DueAt != billDue {
		t.Fatalf("capped %+v %v", capped, err)
	}
}

func TestAReversedBillReadsAsReversedWithNothingOutstanding(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	made := f.bill(t, rec, "a", 400, billDue)
	f.postAt(t, f.sales, rec, 400, accounting.Entry{ReversesEntryID: made.ID, DocumentKind: "bill", DocumentID: "rev-1"})

	got, err := f.repo.GetBill(f.bg, made.ID)
	if err != nil || got.Status != "reversed" || got.Outstanding != 0 {
		t.Fatalf("bill %+v %v", got, err)
	}
	all, err := f.repo.ListBills(f.bg, accounting.BillFilter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("list %+v %v", all, err)
	}
}

func TestABillIsRefusedWithoutADueDateOrTheRightAccounts(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	cases := map[string]accounting.BillInput{
		"no due date":         {IncomeAccountID: f.sales.ID, ReceivableAccountID: rec.ID, Amount: 10, BilledBy: "c"},
		"bad due date":        {IncomeAccountID: f.sales.ID, ReceivableAccountID: rec.ID, Amount: 10, DueAt: "soon", BilledBy: "c"},
		"no biller":           {IncomeAccountID: f.sales.ID, ReceivableAccountID: rec.ID, Amount: 10, DueAt: billDue},
		"zero amount":         {IncomeAccountID: f.sales.ID, ReceivableAccountID: rec.ID, DueAt: billDue, BilledBy: "c"},
		"expense for income":  {IncomeAccountID: f.rent.ID, ReceivableAccountID: rec.ID, Amount: 10, DueAt: billDue, BilledBy: "c"},
		"income for receipts": {IncomeAccountID: f.sales.ID, ReceivableAccountID: f.sales.ID, Amount: 10, DueAt: billDue, BilledBy: "c"},
	}
	for name, in := range cases {
		if _, err := f.repo.CreateBill(f.bg, in); !errors.Is(err, accounting.ErrInvalid) {
			t.Errorf("%s: err %v", name, err)
		}
	}
	missing := accounting.BillInput{IncomeAccountID: "nope", ReceivableAccountID: rec.ID, Amount: 10, DueAt: billDue, BilledBy: "c"}
	if _, err := f.repo.CreateBill(f.bg, missing); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("missing account: err %v", err)
	}
}

func TestOnlyBillsReadAsBillsAndAnEntryWithoutADueDateDoesNot(t *testing.T) {
	f := newStoreFixture(t)
	plain := f.postAt(t, f.cash, f.sales, 50, accounting.Entry{})
	if _, err := f.repo.GetBill(f.bg, plain.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("plain entry err %v", err)
	}
	if _, err := f.repo.GetBill(f.bg, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing err %v", err)
	}
	if _, err := f.repo.ListBills(f.bg, accounting.BillFilter{Status: "late"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("status err %v", err)
	}
	if _, err := f.repo.ListBills(f.bg, accounting.BillFilter{AsOf: "tomorrow"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("as_of err %v", err)
	}
}
