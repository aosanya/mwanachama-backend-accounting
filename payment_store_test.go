package accounting_test

import (
	"context"
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func (f storeFixture) markPaid(t *testing.T, b accounting.Bill, amount int64) accounting.Payment {
	t.Helper()
	p, err := f.repo.MarkBillPaid(f.bg, accounting.MarkPaidInput{BillID: b.ID, CashAccountID: f.cash.ID, Amount: amount, PaidBy: "clerk-2"})
	if err != nil {
		t.Fatalf("MarkBillPaid: %v", err)
	}
	return p
}

func TestMarkingABillPaidPostsCashAgainstTheReceivableNamingTheBill(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-7", 900, billDue)

	p := f.markPaid(t, b, 0)
	if p.Kind != "bill" || p.BillID != b.ID || p.Amount != 900 || p.PaidBy != "clerk-2" || p.Payer != "stall-7" ||
		p.DebitAccountID != f.cash.ID || p.CreditAccountID != rec.ID {
		t.Fatalf("payment %+v", p)
	}
	entries, err := f.repo.ListEntries(f.bg, f.cash.ID, 0)
	if err != nil || len(entries) != 1 || entries[0].SettlesEntryID != b.ID || entries[0].ID != p.ID {
		t.Fatalf("entries %+v %v", entries, err)
	}
	got, err := f.repo.GetBill(f.bg, b.ID)
	if err != nil || got.Status != "settled" || got.Outstanding != 0 || got.Settled != 900 {
		t.Fatalf("bill %+v %v", got, err)
	}
}

func TestAPartPaymentLeavesTheBillOpenAndTheRestCanBePaid(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-7", 900, "2999-01-01T00:00:00Z")
	f.markPaid(t, b, 300)

	got, _ := f.repo.GetBill(f.bg, b.ID)
	if got.Status != "open" || got.Outstanding != 600 {
		t.Fatalf("bill %+v", got)
	}
	f.markPaid(t, b, 0)
	got, _ = f.repo.GetBill(f.bg, b.ID)
	if got.Status != "settled" || got.Settled != 900 {
		t.Fatalf("bill %+v", got)
	}
}

func TestMarkingPaidIsRefusedForASettledOrReversedBillAndBadInput(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	settled := f.bill(t, rec, "a", 100, billDue)
	f.markPaid(t, settled, 0)
	reversed := f.bill(t, rec, "b", 400, billDue)
	f.postAt(t, f.sales, rec, 400, accounting.Entry{ReversesEntryID: reversed.ID, DocumentKind: "bill", DocumentID: "rev-1"})
	open := f.bill(t, rec, "c", 500, billDue)

	refused := []accounting.MarkPaidInput{
		{BillID: settled.ID, CashAccountID: f.cash.ID, PaidBy: "x"},
		{BillID: reversed.ID, CashAccountID: f.cash.ID, PaidBy: "x"},
		{BillID: open.ID, CashAccountID: f.cash.ID, Amount: 501, PaidBy: "x"},
		{BillID: open.ID, CashAccountID: f.cash.ID, Amount: -1, PaidBy: "x"},
		{BillID: open.ID, CashAccountID: f.rent.ID, PaidBy: "x"},
		{BillID: open.ID, CashAccountID: f.cash.ID, PaidBy: " "},
		{BillID: open.ID, CashAccountID: f.cash.ID, PaidBy: "x", PaidOn: "yesterday"},
		{BillID: open.ID, PaidBy: "x"},
		{CashAccountID: f.cash.ID, PaidBy: "x"},
	}
	for _, in := range refused {
		if _, err := f.repo.MarkBillPaid(f.bg, in); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("MarkBillPaid(%+v) = %v, want ErrInvalid", in, err)
		}
	}
	if _, err := f.repo.MarkBillPaid(f.bg, accounting.MarkPaidInput{BillID: "missing", CashAccountID: f.cash.ID, PaidBy: "x"}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown bill = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.MarkBillPaid(f.bg, accounting.MarkPaidInput{BillID: open.ID, CashAccountID: "missing", PaidBy: "x"}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown cash account = %v, want ErrNotFound", err)
	}
	left, _ := f.repo.GetBill(f.bg, open.ID)
	if left.Outstanding != 500 {
		t.Fatalf("a refused payment still moved the bill: %+v", left)
	}
}

func TestPaymentsListBillPaymentsFilteredByBillAndPayerAndSkipReversedOnes(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	a := f.bill(t, rec, "stall-1", 100, billDue)
	b := f.bill(t, rec, "stall-2", 200, billDue)
	pa := f.markPaid(t, a, 0)
	pb := f.markPaid(t, b, 50)
	plain := f.postAt(t, f.rent, f.cash, 7, accounting.Entry{DocumentKind: "voucher", DocumentID: "v-1"})
	f.postAt(t, f.cash, f.rent, 7, accounting.Entry{SettlesEntryID: plain.ID, DocumentKind: "receipt", DocumentID: "r-1"})

	all, err := f.repo.ListPayments(f.bg, accounting.PaymentFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all %+v %v", all, err)
	}
	byBill, _ := f.repo.ListPayments(f.bg, accounting.PaymentFilter{BillID: b.ID})
	if len(byBill) != 1 || byBill[0].ID != pb.ID || byBill[0].Amount != 50 {
		t.Fatalf("by bill %+v", byBill)
	}
	byPayer, _ := f.repo.ListPayments(f.bg, accounting.PaymentFilter{Payer: "stall-1"})
	if len(byPayer) != 1 || byPayer[0].ID != pa.ID {
		t.Fatalf("by payer %+v", byPayer)
	}
	capped, _ := f.repo.ListPayments(f.bg, accounting.PaymentFilter{Limit: 1})
	if len(capped) != 1 {
		t.Fatalf("capped %+v", capped)
	}

	f.postAt(t, rec, f.cash, 100, accounting.Entry{ReversesEntryID: pa.ID, DocumentKind: "payment", DocumentID: "rev-pay"})
	after, _ := f.repo.ListPayments(f.bg, accounting.PaymentFilter{})
	if len(after) != 1 || after[0].ID != pb.ID {
		t.Fatalf("after reversal %+v", after)
	}
	if _, err := f.repo.ListPayments(f.bg, accounting.PaymentFilter{Kind: "cheque"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("bad kind = %v", err)
	}
}

func TestPaymentsAlsoListTheMilestonePaymentCertificates(t *testing.T) {
	f := newContractFixture(t)
	ms := f.milestone(t, 1000)
	c, err := f.repo.CertifyPayment(context.Background(), f.certificate(ms.ID, 400, 1000, futureInstant))
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.ListPayments(context.Background(), accounting.PaymentFilter{})
	if err != nil || len(got) != 1 {
		t.Fatalf("payments %+v %v", got, err)
	}
	p := got[0]
	if p.Kind != "certificate" || p.ID != c.PaymentEntryID || p.Amount != 360 || p.CertificateID != c.ID ||
		p.MilestoneID != ms.ID || p.ContractID != "contract-7" || p.DebitAccountID != f.ward.ID || p.CreditAccountID != f.contactor.ID || p.PaidBy != "engineer-1" {
		t.Fatalf("payment %+v", p)
	}
	if bills, _ := f.repo.ListPayments(context.Background(), accounting.PaymentFilter{Kind: "bill"}); len(bills) != 0 {
		t.Fatalf("bill kind %+v", bills)
	}
	if certs, _ := f.repo.ListPayments(context.Background(), accounting.PaymentFilter{Kind: "certificate"}); len(certs) != 1 {
		t.Fatalf("certificate kind %+v", certs)
	}
	if byBill, _ := f.repo.ListPayments(context.Background(), accounting.PaymentFilter{BillID: "b"}); len(byBill) != 0 {
		t.Fatalf("by bill %+v", byBill)
	}
}

func TestAPaymentCarriesHowItWasPaidAndItsReference(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-7", 900, "2999-01-01T00:00:00Z")

	p, err := f.repo.MarkBillPaid(f.bg, accounting.MarkPaidInput{BillID: b.ID, CashAccountID: f.cash.ID, Amount: 300, Method: "bank transfer", Reference: "TR-2026-77", PaidBy: "clerk-2"})
	if err != nil || p.Method != "bank transfer" || p.Reference != "TR-2026-77" {
		t.Fatalf("payment %+v %v", p, err)
	}
	listed, err := f.repo.ListPayments(f.bg, accounting.PaymentFilter{BillID: b.ID})
	if err != nil || len(listed) != 1 || listed[0].Method != "bank transfer" || listed[0].Reference != "TR-2026-77" || listed[0].Payer != "stall-7" {
		t.Fatalf("listed %+v %v", listed, err)
	}
}

func TestAPaymentDatedInTheFutureIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-7", 900, "2999-01-01T00:00:00Z")

	_, err := f.repo.MarkBillPaid(f.bg, accounting.MarkPaidInput{BillID: b.ID, CashAccountID: f.cash.ID, Amount: 100, PaidOn: "2999-01-01T00:00:00Z", PaidBy: "clerk-2"})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err %v", err)
	}
}

func TestReversingAPaymentRestoresWhatTheBillOwesAndLeavesTheOriginalInTheLedger(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-7", 900, "2999-01-01T00:00:00Z")
	paid := f.markPaid(t, b, 400)

	reversed, err := f.repo.ReversePayment(f.bg, accounting.ReversePaymentInput{PaymentID: paid.ID, Narration: "Entered twice", ReversedBy: "clerk-3"})
	if err != nil || reversed.ID != paid.ID {
		t.Fatalf("reversed %+v %v", reversed, err)
	}
	got, err := f.repo.GetBill(f.bg, b.ID)
	if err != nil || got.Outstanding != 900 || got.Settled != 0 || got.Status != "open" {
		t.Fatalf("bill %+v %v", got, err)
	}
	listed, err := f.repo.ListPayments(f.bg, accounting.PaymentFilter{BillID: b.ID})
	if err != nil || len(listed) != 0 {
		t.Fatalf("listed %+v %v", listed, err)
	}
	entries, err := f.repo.ListEntries(f.bg, f.cash.ID, 0)
	if err != nil || len(entries) != 2 || entries[1].ReversesEntryID != paid.ID || entries[1].ActorID != "clerk-3" || entries[1].Narration != "Entered twice" {
		t.Fatalf("entries %+v %v", entries, err)
	}
}

func TestAPaymentIsReversedOnceAndOnlyAPaymentIsReversed(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-7", 900, "2999-01-01T00:00:00Z")
	paid := f.markPaid(t, b, 400)
	if _, err := f.repo.ReversePayment(f.bg, accounting.ReversePaymentInput{PaymentID: paid.ID, ReversedBy: "clerk-3"}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.repo.ReversePayment(f.bg, accounting.ReversePaymentInput{PaymentID: paid.ID, ReversedBy: "clerk-3"}); !errors.Is(err, accounting.ErrAlreadyReversed) {
		t.Fatalf("twice: %v", err)
	}
	if _, err := f.repo.ReversePayment(f.bg, accounting.ReversePaymentInput{PaymentID: b.ID, ReversedBy: "clerk-3"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("a bill is not a payment: %v", err)
	}
	if _, err := f.repo.ReversePayment(f.bg, accounting.ReversePaymentInput{PaymentID: "nope", ReversedBy: "clerk-3"}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := f.repo.ReversePayment(f.bg, accounting.ReversePaymentInput{PaymentID: paid.ID}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("no reversed_by: %v", err)
	}
}

func TestAPaymentAboveWhatIsOwedIsRefusedUnlessTheOwnerAllowsIt(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-9", 500, billDue)
	over := accounting.MarkPaidInput{BillID: b.ID, CashAccountID: f.cash.ID, Amount: 700, PaidBy: "clerk-2"}

	if _, err := f.repo.MarkBillPaid(f.bg, over); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("overpayment with no setting = %v, want ErrInvalid", err)
	}
	f.repo.UseOverpaymentAllowed(func(context.Context) (bool, error) { return false, nil })
	if _, err := f.repo.MarkBillPaid(f.bg, over); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("overpayment refused by the setting = %v, want ErrInvalid", err)
	}
	f.repo.UseOverpaymentAllowed(func(context.Context) (bool, error) { return true, nil })
	p, err := f.repo.MarkBillPaid(f.bg, over)
	if err != nil || p.Amount != 700 {
		t.Fatalf("overpayment allowed = %+v %v", p, err)
	}
	got, err := f.repo.GetBill(f.bg, b.ID)
	if err != nil || got.Status != "settled" || got.Outstanding != 0 || got.Settled != 700 {
		t.Fatalf("an overpaid bill reads %+v %v, want settled with nothing outstanding", got, err)
	}
}

func TestAnUnreadableOverpaymentSettingKeepsTheOverpaymentRefused(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	b := f.bill(t, rec, "stall-9", 500, billDue)
	f.repo.UseOverpaymentAllowed(func(context.Context) (bool, error) { return false, errors.New("setting unreadable") })
	if _, err := f.repo.MarkBillPaid(f.bg, accounting.MarkPaidInput{BillID: b.ID, CashAccountID: f.cash.ID, Amount: 700, PaidBy: "x"}); err == nil {
		t.Fatalf("overpayment went through with an unreadable setting")
	}
}
