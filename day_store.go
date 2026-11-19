package accounting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	roleDayAllowance = "day_allowance"
	roleDayEntry     = "day_entry"

	RoleDayAllowance = roleDayAllowance
	RoleDayEntry     = roleDayEntry

	hundredthsPerDay = 100
	dayRowCeiling    = 10000
	dayCeiling       = hundredthsPerDay
)

type (
	DayAllowance   = models.DayAllowance
	DayEntry       = models.DayEntry
	DayEntryStatus = models.DayEntryStatus
	DaysLine       = models.DaysLine
	DaysBalance    = models.DaysBalance
)

const (
	DayEntrySubmitted = models.DayEntrySubmitted
	DayEntryApproved  = models.DayEntryApproved
	DayEntryRejected  = models.DayEntryRejected
)

var ErrOverDaysBought = errors.New("accounting: the approved days would pass the days bought")

var ErrDayEntryDecided = errors.New("accounting: that day entry has already been decided")

var ErrOwnDayEntry = errors.New("accounting: a day entry is not approved by the person who submitted it or worked it")

var ErrDateOverfull = errors.New("accounting: that date would hold more than one day for the consultant")

type DayRepository interface {
	AddDayAllowance(ctx context.Context, a DayAllowance) (DayAllowance, error)

	ReplaceDayAllowance(ctx context.Context, a DayAllowance) (DayAllowance, error)

	ListDayAllowances(ctx context.Context, project string, limit int) ([]DayAllowance, error)

	SubmitDayEntry(ctx context.Context, e DayEntry) (DayEntry, error)

	ApproveDayEntry(ctx context.Context, entryID, decidedBy string) (DayEntry, error)

	RejectDayEntry(ctx context.Context, entryID, decidedBy string) (DayEntry, error)

	ListDayEntries(ctx context.Context, project, consultant, status string, limit int) ([]DayEntry, error)

	DaysBalance(ctx context.Context, project string) (DaysBalance, error)
}

func addDayCarriers(s *spec.Spec, carriers map[string]any) {
	if _, ok := s.ByRole(roleDayAllowance); ok {
		carriers[roleDayAllowance] = models.DayAllowance{}
	}
	if _, ok := s.ByRole(roleDayEntry); ok {
		carriers[roleDayEntry] = models.DayEntry{}
	}
}

func (m *ledgerStore) keepsDays() error {
	_, allowances := m.st.Spec().ByRole(roleDayAllowance)
	_, entries := m.st.Spec().ByRole(roleDayEntry)
	if !allowances || !entries {
		return fmt.Errorf("%w: this domain keeps no days", ErrInvalid)
	}
	return nil
}

func dayFee(hundredths, rate int64) int64 {
	return (hundredths*rate + hundredthsPerDay/2) / hundredthsPerDay
}

func feeFits(hundredths, rate int64) bool {
	return rate == 0 || hundredths <= (math.MaxInt64-hundredthsPerDay/2)/rate
}

func normalisedName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func workedOnDay(day string) error {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil || parsed.Format("2006-01-02") != day {
		return fmt.Errorf("%w: worked_on %q is not a day written YYYY-MM-DD", ErrInvalid, day)
	}
	return nil
}

func checkDayAllowance(a DayAllowance) error {
	if a.DaysBought <= 0 {
		return fmt.Errorf("%w: days_bought %d is not positive", ErrInvalid, a.DaysBought)
	}
	if a.Rate < 0 {
		return fmt.Errorf("%w: rate %d is negative", ErrInvalid, a.Rate)
	}
	if !feeFits(a.DaysBought, a.Rate) {
		return fmt.Errorf("%w: %d hundredths of a day at a rate of %d is a fee too large to count", ErrInvalid, a.DaysBought, a.Rate)
	}
	if a.PeriodStart == "" && a.PeriodEnd == "" {
		return nil
	}
	_, _, err := parsePeriod(a.PeriodStart, a.PeriodEnd)
	return err
}

func (m *ledgerStore) dayAllowancesOf(ctx context.Context, tx *gorm.DB, project, consultant string) ([]DayAllowance, error) {
	q := m.on(ctx, tx, roleDayAllowance).Where("project = ?", project)
	if consultant != "" {
		q = q.Where("consultant = ?", consultant)
	}
	return specstore.List[DayAllowance](m.st, q.Order("set_at").Limit(dayRowCeiling), roleDayAllowance)
}

func currentAllowances(rows []DayAllowance) map[string]DayAllowance {
	replaced := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.ReplacesID != "" {
			replaced[r.ReplacesID] = true
		}
	}
	current := map[string]DayAllowance{}
	for _, r := range rows {
		if !replaced[r.ID] {
			current[r.Consultant] = r
		}
	}
	return current
}

func (m *ledgerStore) writeDayAllowance(ctx context.Context, a DayAllowance) (DayAllowance, error) {
	if err := m.keepsDays(); err != nil {
		return DayAllowance{}, err
	}
	a.Consultant = normalisedName(a.Consultant)
	if err := m.checks(roleDayAllowance, a); err != nil {
		return DayAllowance{}, err
	}
	if err := checkDayAllowance(a); err != nil {
		return DayAllowance{}, err
	}

	var out DayAllowance
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := m.dayAllowancesOf(ctx, tx, a.Project, a.Consultant)
		if err != nil {
			return err
		}
		current, held := currentAllowances(rows)[a.Consultant]
		if a.ReplacesID == "" && held {
			return fmt.Errorf("%w: %s already holds days on %s; replace that allowance, naming the reason", ErrInvalid, a.Consultant, a.Project)
		}
		if a.ReplacesID != "" {
			if !held || current.ID != a.ReplacesID {
				return fmt.Errorf("%w: %s is not the current allowance of %s on %s", ErrInvalid, a.ReplacesID, a.Consultant, a.Project)
			}
			if strings.TrimSpace(a.Reason) == "" {
				return fmt.Errorf("%w: a replacing allowance says why the days changed", ErrInvalid)
			}
			approved, err := m.approvedHundredths(ctx, tx, a.Project, a.Consultant)
			if err != nil {
				return err
			}
			if a.DaysBought < approved {
				return fmt.Errorf("%w: %s has %d hundredths approved on %s and the replacing allowance holds %d",
					ErrOverDaysBought, a.Consultant, approved, a.Project, a.DaysBought)
			}
		}
		a.ID = newID()
		a.SetAt = m.now()
		row, err := encode(m.st.Object(roleDayAllowance), a)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleDayAllowance).Create(row).Error; err != nil {
			return err
		}
		out = a
		return nil
	})
	if err != nil {
		return DayAllowance{}, err
	}
	return out, nil
}

func (m *ledgerStore) AddDayAllowance(ctx context.Context, a DayAllowance) (DayAllowance, error) {
	if a.ReplacesID != "" {
		return DayAllowance{}, fmt.Errorf("%w: a new allowance replaces nothing; use the replace call", ErrInvalid)
	}
	return m.writeDayAllowance(ctx, a)
}

func (m *ledgerStore) ReplaceDayAllowance(ctx context.Context, a DayAllowance) (DayAllowance, error) {
	if a.ReplacesID == "" {
		return DayAllowance{}, fmt.Errorf("%w: a replacing allowance names the row it replaces", ErrInvalid)
	}
	var replaced DayAllowance
	err := m.take(m.on(ctx, m.db, roleDayAllowance).Where("id = ?", a.ReplacesID), roleDayAllowance, &replaced, ErrNotFound)
	if err != nil {
		return DayAllowance{}, err
	}
	a.Project, a.Consultant = replaced.Project, replaced.Consultant
	return m.writeDayAllowance(ctx, a)
}

func (m *ledgerStore) ListDayAllowances(ctx context.Context, project string, limit int) ([]DayAllowance, error) {
	if err := m.keepsDays(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(project) == "" {
		return nil, fmt.Errorf("%w: a list of allowances names its project", ErrInvalid)
	}
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleDayAllowance).Where("project = ?", project).Order("set_at").Limit(limit)
	return specstore.List[DayAllowance](m.st, q, roleDayAllowance)
}

func (m *ledgerStore) SubmitDayEntry(ctx context.Context, e DayEntry) (DayEntry, error) {
	if err := m.keepsDays(); err != nil {
		return DayEntry{}, err
	}
	e.Status = DayEntrySubmitted
	e.Consultant = normalisedName(e.Consultant)
	if err := m.checks(roleDayEntry, e); err != nil {
		return DayEntry{}, err
	}
	if e.Days <= 0 || e.Days > dayCeiling {
		return DayEntry{}, fmt.Errorf("%w: days %d is not between 1 and %d hundredths", ErrInvalid, e.Days, dayCeiling)
	}
	if err := workedOnDay(e.WorkedOn); err != nil {
		return DayEntry{}, err
	}
	e.ID = newID()
	e.SubmittedAt = m.now()
	e.Rate, e.DecidedBy, e.DecidedAt = 0, "", ""
	row, err := encode(m.st.Object(roleDayEntry), e)
	if err != nil {
		return DayEntry{}, err
	}
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		onDate, err := m.heldOnDate(ctx, tx, e)
		if err != nil {
			return err
		}
		if onDate+e.Days > dayCeiling {
			return fmt.Errorf("%w: %s holds %d of %d hundredths on %s and this entry adds %d",
				ErrDateOverfull, e.Consultant, onDate, dayCeiling, e.WorkedOn, e.Days)
		}
		return m.on(ctx, tx, roleDayEntry).Create(row).Error
	})
	if err != nil {
		return DayEntry{}, err
	}
	return e, nil
}

func (m *ledgerStore) heldOnDate(ctx context.Context, tx *gorm.DB, e DayEntry) (int64, error) {
	var total int64
	err := m.on(ctx, tx, roleDayEntry).
		Where("project = ? AND consultant = ? AND worked_on = ? AND status <> ?", e.Project, e.Consultant, e.WorkedOn, string(DayEntryRejected)).
		Select("coalesce(sum(days), 0)").Scan(&total).Error
	return total, err
}

func (m *ledgerStore) approvedHundredths(ctx context.Context, tx *gorm.DB, project, consultant string) (int64, error) {
	var total int64
	err := m.on(ctx, tx, roleDayEntry).
		Where("project = ? AND consultant = ? AND status = ?", project, consultant, string(DayEntryApproved)).
		Select("coalesce(sum(days), 0)").Scan(&total).Error
	return total, err
}

func (m *ledgerStore) decideDayEntry(ctx context.Context, entryID, decidedBy string, status DayEntryStatus) (DayEntry, error) {
	if err := m.keepsDays(); err != nil {
		return DayEntry{}, err
	}
	if strings.TrimSpace(decidedBy) == "" {
		return DayEntry{}, fmt.Errorf("%w: a decision names no deciding person", ErrInvalid)
	}
	var out DayEntry
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e DayEntry
		if err := m.take(m.on(ctx, tx, roleDayEntry).Where("id = ?", entryID), roleDayEntry, &e, ErrNotFound); err != nil {
			return err
		}
		if e.Status != DayEntrySubmitted {
			return ErrDayEntryDecided
		}
		if status == DayEntryApproved {
			if madeBy(e, decidedBy) {
				return ErrOwnDayEntry
			}
			allowance, err := m.withinDaysBought(ctx, tx, e)
			if err != nil {
				return err
			}
			e.Rate = allowance.Rate
		}
		e.Status, e.DecidedBy, e.DecidedAt = status, decidedBy, m.now()
		row, err := encode(m.st.Object(roleDayEntry), e)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleDayEntry).Where("id = ?", e.ID).
			Updates(mutableColumns(m.st.Object(roleDayEntry), row)).Error; err != nil {
			return err
		}
		out = e
		return nil
	})
	if err != nil {
		return DayEntry{}, err
	}
	return out, nil
}

func madeBy(e DayEntry, person string) bool {
	who := normalisedName(person)
	return who == normalisedName(e.SubmittedBy) || who == normalisedName(e.Consultant)
}

func (m *ledgerStore) withinDaysBought(ctx context.Context, tx *gorm.DB, e DayEntry) (DayAllowance, error) {
	rows, err := m.dayAllowancesOf(ctx, tx, e.Project, e.Consultant)
	if err != nil {
		return DayAllowance{}, err
	}
	allowance, held := currentAllowances(rows)[e.Consultant]
	if !held {
		return DayAllowance{}, fmt.Errorf("%w: %s holds no days on %s", ErrOverDaysBought, e.Consultant, e.Project)
	}
	approved, err := m.approvedHundredths(ctx, tx, e.Project, e.Consultant)
	if err != nil {
		return DayAllowance{}, err
	}
	if approved+e.Days > allowance.DaysBought {
		return DayAllowance{}, fmt.Errorf("%w: %s has %d of %d hundredths approved on %s and this entry adds %d",
			ErrOverDaysBought, e.Consultant, approved, allowance.DaysBought, e.Project, e.Days)
	}
	return allowance, nil
}

func (m *ledgerStore) ApproveDayEntry(ctx context.Context, entryID, decidedBy string) (DayEntry, error) {
	return m.decideDayEntry(ctx, entryID, decidedBy, DayEntryApproved)
}

func (m *ledgerStore) RejectDayEntry(ctx context.Context, entryID, decidedBy string) (DayEntry, error) {
	return m.decideDayEntry(ctx, entryID, decidedBy, DayEntryRejected)
}

func (m *ledgerStore) ListDayEntries(ctx context.Context, project, consultant, status string, limit int) ([]DayEntry, error) {
	if err := m.keepsDays(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(project) == "" {
		return nil, fmt.Errorf("%w: a list of day entries names its project", ErrInvalid)
	}
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleDayEntry).Where("project = ?", project)
	if consultant != "" {
		q = q.Where("consultant = ?", normalisedName(consultant))
	}
	if status != "" {
		if !declares([]string{string(DayEntrySubmitted), string(DayEntryApproved), string(DayEntryRejected)}, status) {
			return nil, fmt.Errorf("%w: status %q is not one of submitted, approved, rejected", ErrInvalid, status)
		}
		q = q.Where("status = ?", status)
	}
	return specstore.List[DayEntry](m.st, q.Order("worked_on, submitted_at").Limit(limit), roleDayEntry)
}

type dayTotal struct {
	Consultant string
	Status     string
	Rate       int64
	Total      int64
}

func (m *ledgerStore) dayTotals(ctx context.Context, project string) ([]dayTotal, error) {
	var totals []dayTotal
	err := m.q(ctx, roleDayEntry).
		Select("consultant, status, rate, sum(days) as total").
		Where("project = ?", project).
		Group("consultant, status, rate").
		Scan(&totals).Error
	return totals, err
}

func (m *ledgerStore) DaysBalance(ctx context.Context, project string) (DaysBalance, error) {
	if err := m.keepsDays(); err != nil {
		return DaysBalance{}, err
	}
	if strings.TrimSpace(project) == "" {
		return DaysBalance{}, fmt.Errorf("%w: a days balance names its project", ErrInvalid)
	}
	rows, err := m.dayAllowancesOf(ctx, m.db, project, "")
	if err != nil {
		return DaysBalance{}, err
	}
	totals, err := m.dayTotals(ctx, project)
	if err != nil {
		return DaysBalance{}, err
	}

	byConsultant := map[string]*DaysLine{}
	line := func(consultant string) *DaysLine {
		if byConsultant[consultant] == nil {
			byConsultant[consultant] = &DaysLine{Consultant: consultant}
		}
		return byConsultant[consultant]
	}
	for consultant, a := range currentAllowances(rows) {
		l := line(consultant)
		l.Position, l.Rate, l.Bought = a.Position, a.Rate, a.DaysBought
	}
	for _, t := range totals {
		switch DayEntryStatus(t.Status) {
		case DayEntrySubmitted:
			line(t.Consultant).Submitted += t.Total
		case DayEntryApproved:
			l := line(t.Consultant)
			l.Approved += t.Total
			l.Fee += dayFee(t.Total, t.Rate)
		}
	}

	balance := DaysBalance{Project: project, Lines: make([]DaysLine, 0, len(byConsultant))}
	for _, l := range byConsultant {
		l.Left = l.Bought - l.Approved
		l.FeeLeft = dayFee(l.Left, l.Rate)
		l.FeeCeiling = l.Fee + l.FeeLeft
		balance.Lines = append(balance.Lines, *l)
	}
	sort.Slice(balance.Lines, func(i, j int) bool { return balance.Lines[i].Consultant < balance.Lines[j].Consultant })
	for _, l := range balance.Lines {
		balance.Total.Bought += l.Bought
		balance.Total.Submitted += l.Submitted
		balance.Total.Approved += l.Approved
		balance.Total.Left += l.Left
		balance.Total.FeeCeiling += l.FeeCeiling
		balance.Total.Fee += l.Fee
		balance.Total.FeeLeft += l.FeeLeft
	}
	return balance, nil
}
