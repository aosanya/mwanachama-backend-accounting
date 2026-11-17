package accounting

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const roleFigure = "figure"

const RoleFigure = roleFigure

type Figure = models.Figure

var ErrFigureTaken = errors.New("accounting: that subject already has an entry for this figure on this day")

var figureNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

var figureValuePattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

const figureValueDigits = 30

func (m *ledgerStore) keepsFigures() bool {
	_, ok := m.st.Spec().ByRole(roleFigure)
	return ok
}

func figureDay(day string) error {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil || parsed.Format("2006-01-02") != day {
		return fmt.Errorf("%w: on_day %q is not a day written YYYY-MM-DD", ErrInvalid, day)
	}
	return nil
}

func figureValue(value string) error {
	digits := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	if !figureValuePattern.MatchString(value) || digits > figureValueDigits {
		return fmt.Errorf("%w: value %q is not a decimal number of at most %d digits written with a dot and no sign other than a minus",
			ErrInvalid, value, figureValueDigits)
	}
	return nil
}

func (m *ledgerStore) checkFigure(f Figure) error {
	if err := m.checks(roleFigure, f); err != nil {
		return err
	}
	if !figureNamePattern.MatchString(f.Figure) {
		return fmt.Errorf("%w: figure %q is lower-case letters, digits and underscores, starting with a letter", ErrInvalid, f.Figure)
	}
	if err := figureDay(f.OnDay); err != nil {
		return err
	}
	return figureValue(f.Value)
}

func (m *ledgerStore) figureOnDay(ctx context.Context, tx *gorm.DB, f Figure) (Figure, error) {
	var out Figure
	err := m.take(m.on(ctx, tx, roleFigure).
		Where("subject_id = ? AND figure = ? AND on_day = ?", f.SubjectID, f.Figure, f.OnDay),
		roleFigure, &out, ErrNotFound)
	return out, err
}

func (m *ledgerStore) AppendFigure(ctx context.Context, f Figure) (Figure, error) {
	if !m.keepsFigures() {
		return Figure{}, fmt.Errorf("%w: this domain keeps no record of figures", ErrInvalid)
	}
	if err := m.checkFigure(f); err != nil {
		return Figure{}, err
	}

	var out Figure
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := m.figureOnDay(ctx, tx, f)
		switch {
		case err == nil:
			return ErrFigureTaken
		case !errors.Is(err, ErrNotFound):
			return err
		}

		f.ID = newID()
		f.RecordedAt = m.now()
		row, err := encode(m.st.Object(roleFigure), f)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleFigure).Create(row).Error; err != nil {
			if _, reread := m.figureOnDay(ctx, tx, f); reread == nil {
				return ErrFigureTaken
			}
			return err
		}
		out = f
		return nil
	})
	if err != nil {
		return Figure{}, err
	}
	return out, nil
}

func (m *ledgerStore) ListFigureHistory(ctx context.Context, subjectID, figure string, limit int) ([]Figure, error) {
	if !m.keepsFigures() {
		return nil, fmt.Errorf("%w: this domain keeps no record of figures", ErrInvalid)
	}
	if subjectID == "" || figure == "" {
		return nil, fmt.Errorf("%w: a figure's history names its subject and its figure", ErrInvalid)
	}
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleFigure).
		Where("subject_id = ? AND figure = ?", subjectID, figure).
		Order("on_day desc, recorded_at desc").
		Limit(limit)
	return specstore.List[Figure](m.st, q, roleFigure)
}
