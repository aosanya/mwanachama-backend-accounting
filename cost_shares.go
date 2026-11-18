package accounting

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const NeighbourhoodKey = "neighbourhood"

const wholeShare = 100

func checkCostShares(shares map[string]int64) error {
	if len(shares) == 0 {
		return nil
	}
	var sum int64
	for name, percent := range shares {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: a cost share needs a neighbourhood name", ErrInvalid)
		}
		if err := checkDimensionValue(NeighbourhoodKey, name); err != nil {
			return err
		}
		if percent <= 0 || percent > wholeShare {
			return fmt.Errorf("%w: the share of %q is %d percent, it must be a whole number from 1 to %d", ErrInvalid, name, percent, wholeShare)
		}
		sum += percent
	}
	if sum != wholeShare {
		return fmt.Errorf("%w: the cost shares add up to %d percent, they must add up to %d", ErrInvalid, sum, wholeShare)
	}
	return nil
}

type sharedLine struct {
	accountID string
	from, to  time.Time
	shares    map[string]int64
}

type sharedLines []sharedLine

func sharedLinesOf(budgets []Budget) sharedLines {
	out := sharedLines{}
	for _, b := range budgets {
		if len(b.CostShares) == 0 {
			continue
		}
		from, to, err := parsePeriod(b.PeriodStart, b.PeriodEnd)
		if err != nil {
			continue
		}
		out = append(out, sharedLine{accountID: b.AccountID, from: from, to: to, shares: b.CostShares})
	}
	return out
}

func (lines sharedLines) sharesOf(e Entry, selected accountSelection) (map[string]int64, bool) {
	for _, line := range lines {
		touches := line.accountID == e.DebitAccountID || line.accountID == e.CreditAccountID
		if touches && selected(line.accountID) && withinPeriod(e, line.from, line.to) {
			return line.shares, true
		}
	}
	return nil, false
}

type portion struct {
	value  string
	amount int64
}

func apportion(total int64, shares map[string]int64) []portion {
	names := make([]string, 0, len(shares))
	for name := range shares {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]portion, 0, len(names))
	var given int64
	largest := 0
	for i, name := range names {
		percent := shares[name]
		amount := total/wholeShare*percent + total%wholeShare*percent/wholeShare
		parts = append(parts, portion{value: name, amount: amount})
		given += amount
		if percent > shares[names[largest]] {
			largest = i
		}
	}
	parts[largest].amount += total - given
	return parts
}
