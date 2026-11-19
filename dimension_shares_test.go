package accounting_test

import (
	"errors"
	"reflect"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func (l townLedger) lineOn(t *testing.T, account accounting.Account, follow bool, shares map[string]int64) accounting.Budget {
	t.Helper()
	b, err := l.repo.SetBudget(t.Context(), accounting.Budget{
		AccountID: account.ID, PeriodStart: januaryOpens, PeriodEnd: februaryOpens,
		Amount:           2000000,
		FollowMilestones: follow, CostShares: shares,
	})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	return b
}

func (l townLedger) neighbourhoodTotals(t *testing.T) []accounting.DimensionTotal {
	t.Helper()
	got, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", string(accounting.AccountTypeExpense), "", "")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	return got.Totals
}

func TestASharedLineSplitsItsSpendingBetweenTheNeighbourhoodsByPercent(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, true, map[string]int64{"Óváros": 60, "Kertváros": 40})
	l.spend(t, l.repair, 1000000, "2026-01-15T09:00:00Z", neighbourhood("Malomsor"))

	want := []accounting.DimensionTotal{
		{Value: "Kertváros", Total: 400000, Entries: 1},
		{Value: "Óváros", Total: 600000, Entries: 1},
	}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestASharedLineCountsSpendingThatNamesNoNeighbourhood(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, true, map[string]int64{"Óváros": 60, "Kertváros": 40})
	l.spend(t, l.repair, 1000, "2026-01-15T09:00:00Z", nil)

	want := []accounting.DimensionTotal{
		{Value: "Kertváros", Total: 400, Entries: 1},
		{Value: "Óváros", Total: 600, Entries: 1},
	}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestALineWithoutSharesKeepsTheNeighbourhoodEachEntryNames(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, false, nil)
	l.spend(t, l.repair, 1000000, "2026-01-15T09:00:00Z", neighbourhood("Malomsor"))

	want := []accounting.DimensionTotal{{Value: "Malomsor", Total: 1000000, Entries: 1}}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestSharesOnlyApplyToSpendingInsideTheLinesPeriod(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, false, map[string]int64{"Óváros": 60, "Kertváros": 40})
	l.spend(t, l.repair, 1000, februaryOpens, neighbourhood("Malomsor"))

	want := []accounting.DimensionTotal{{Value: "Malomsor", Total: 1000, Entries: 1}}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestSharesOnlyApplyToTheAccountOfTheirLine(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, false, map[string]int64{"Óváros": 60, "Kertváros": 40})
	l.spend(t, l.wages, 1000, "2026-01-15T09:00:00Z", neighbourhood("Malomsor"))

	want := []accounting.DimensionTotal{{Value: "Malomsor", Total: 1000, Entries: 1}}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestAShareThatDoesNotDivideEvenlyLosesNothingAndGivesTheRestToTheLargestShare(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, false, map[string]int64{"Óváros": 50, "Kertváros": 30, "Malomsor": 20})
	l.spend(t, l.repair, 5, "2026-01-15T09:00:00Z", nil)

	want := []accounting.DimensionTotal{
		{Value: "Kertváros", Total: 1, Entries: 1},
		{Value: "Malomsor", Total: 1, Entries: 1},
		{Value: "Óváros", Total: 3, Entries: 1},
	}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestAReversalOnASharedLineTakesBackTheSameSplit(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, false, map[string]int64{"Óváros": 60, "Kertváros": 40})
	original := l.spend(t, l.repair, 1000, "2026-01-15T09:00:00Z", nil)
	l.reverse(t, original, "2026-01-16T09:00:00Z")

	want := []accounting.DimensionTotal{
		{Value: "Kertváros", Total: 0, Entries: 2},
		{Value: "Óváros", Total: 0, Entries: 2},
	}
	if got := l.neighbourhoodTotals(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}

func TestSharesThatDoNotSumToOneHundredAreRefused(t *testing.T) {
	for name, shares := range map[string]map[string]int64{
		"short":    {"Óváros": 60, "Kertváros": 30},
		"over":     {"Óváros": 60, "Kertváros": 50},
		"zero":     {"Óváros": 100, "Kertváros": 0},
		"negative": {"Óváros": 120, "Kertváros": -20},
		"unnamed":  {"": 100},
	} {
		t.Run(name, func(t *testing.T) {
			l := openTownLedger(t)
			_, err := l.repo.SetBudget(t.Context(), accounting.Budget{
				AccountID: l.repair.ID, PeriodStart: januaryOpens, PeriodEnd: februaryOpens, Amount: 10, CostShares: shares,
			})
			if !errors.Is(err, accounting.ErrInvalid) {
				t.Fatalf("SetBudget(%v) = %v, want ErrInvalid", shares, err)
			}
		})
	}
}

func TestSettingALineAnswersWithItsMilestoneSwitchAndItsShares(t *testing.T) {
	l := openTownLedger(t)
	set := l.lineOn(t, l.repair, true, map[string]int64{"Óváros": 60, "Kertváros": 40})

	if !set.FollowMilestones || !reflect.DeepEqual(set.CostShares, map[string]int64{"Óváros": 60, "Kertváros": 40}) {
		t.Fatalf("budget %+v", set)
	}
}

func TestANewBudgetLineDoesNotFollowMilestonesAndCountsItsCostInOne(t *testing.T) {
	l := openTownLedger(t)
	set := l.lineOn(t, l.repair, false, nil)

	if set.FollowMilestones || len(set.CostShares) != 0 {
		t.Fatalf("budget %+v", set)
	}
}

func TestSettingTheLineAgainWithoutSharesTurnsSharingOff(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, true, map[string]int64{"Óváros": 60, "Kertváros": 40})
	again := l.lineOn(t, l.repair, false, nil)
	l.spend(t, l.repair, 1000, "2026-01-15T09:00:00Z", neighbourhood("Malomsor"))

	want := []accounting.DimensionTotal{{Value: "Malomsor", Total: 1000, Entries: 1}}
	if got := l.neighbourhoodTotals(t); again.FollowMilestones || len(again.CostShares) != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("budget %+v, totals %+v, want %+v", again, got, want)
	}
}

func TestSharesDoNotChangeTheTotalsOfAnyOtherDimensionKey(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, false, map[string]int64{"Óváros": 60, "Kertváros": 40})
	l.spend(t, l.repair, 1000, "2026-01-15T09:00:00Z", map[string]string{"project": "Fo utca"})

	got, err := l.repo.DimensionTotals(t.Context(), "project", string(accounting.AccountTypeExpense), "", "")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	want := []accounting.DimensionTotal{{Value: "Fo utca", Total: 1000, Entries: 1}}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals %+v, want %+v", got.Totals, want)
	}
}

func TestABudgetLineReadsBackWhetherItFollowsMilestonesAndHowItsCostIsShared(t *testing.T) {
	l := openTownLedger(t)
	set := l.lineOn(t, l.repair, true, map[string]int64{"Óváros": 60, "Kertváros": 40})

	got, err := l.repo.GetBudgetLine(t.Context(), set.ID)
	if err != nil {
		t.Fatalf("GetBudgetLine: %v", err)
	}
	if !got.FollowMilestones || !reflect.DeepEqual(got.CostShares, map[string]int64{"Óváros": 60, "Kertváros": 40}) {
		t.Fatalf("line %+v", got)
	}
}

func TestANewBudgetLineReadsBackAsNotFollowingMilestonesAndNotSharingItsCost(t *testing.T) {
	l := openTownLedger(t)
	set := l.lineOn(t, l.repair, false, nil)

	got, err := l.repo.GetBudgetLine(t.Context(), set.ID)
	if err != nil {
		t.Fatalf("GetBudgetLine: %v", err)
	}
	if got.FollowMilestones || len(got.CostShares) != 0 {
		t.Fatalf("line %+v", got)
	}
}

func TestAnotherSetWithoutSharesReadsBackAsNotSharingItsCost(t *testing.T) {
	l := openTownLedger(t)
	l.lineOn(t, l.repair, true, map[string]int64{"Óváros": 60, "Kertváros": 40})
	again := l.lineOn(t, l.repair, false, nil)

	got, err := l.repo.GetBudgetLine(t.Context(), again.ID)
	if err != nil {
		t.Fatalf("GetBudgetLine: %v", err)
	}
	if got.FollowMilestones || len(got.CostShares) != 0 {
		t.Fatalf("line %+v", got)
	}
}
