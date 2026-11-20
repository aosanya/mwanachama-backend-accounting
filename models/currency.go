package models

const (
	RateScale       int64 = 100_000_000
	NoCurrency            = "XXX"
	RateDaily             = "daily"
	RateMonthly           = "monthly"
	RateAgreed            = "agreed"
	AverageRateKind       = "average"
)

type Currency struct {
	Code     string `json:"code"`
	Decimals int    `json:"decimals"`
}

var currencies = []Currency{
	{"AED", 2}, {"AUD", 2}, {"BIF", 0}, {"CAD", 2}, {"CHF", 2}, {"CNY", 2},
	{"DKK", 2}, {"EUR", 2}, {"GBP", 2}, {"GHS", 2}, {"HUF", 2}, {"INR", 2},
	{"JPY", 0}, {"KES", 2}, {"NGN", 2}, {"NOK", 2}, {"PLN", 2}, {"RWF", 0},
	{"SEK", 2}, {"TZS", 2}, {"UGX", 0}, {"USD", 2}, {"XOF", 0}, {"ZAR", 2},
	{NoCurrency, 2},
}

func Currencies() []Currency {
	out := make([]Currency, len(currencies))
	copy(out, currencies)
	return out
}

func CurrencyOf(code string) (Currency, bool) {
	for _, c := range currencies {
		if c.Code == code {
			return c, true
		}
	}
	return Currency{}, false
}

func IsCurrency(code string) bool {
	_, ok := CurrencyOf(code)
	return ok
}

type ExchangeRate struct {
	ID         string `json:"id"`
	Base       string `json:"base"`
	Counter    string `json:"counter"`
	Rate       int64  `json:"rate"`
	AppliesOn  string `json:"applies_on"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	ReplacesID string `json:"replaces_id,omitempty"`
	RecordedBy string `json:"recorded_by"`
	RecordedAt string `json:"recorded_at"`
}

type RateInput struct {
	Base       string `json:"base"`
	Counter    string `json:"counter"`
	Rate       int64  `json:"rate"`
	AppliesOn  string `json:"applies_on"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	ReplacesID string `json:"replaces_id,omitempty"`
	RecordedBy string `json:"recorded_by"`
}

type ConversionRule string

const (
	RuleRateAtPostingDate  ConversionRule = "posting_date"
	RuleRateOfPostingMonth ConversionRule = "posting_month"
	RuleAverageOverPeriod  ConversionRule = "period_average"
	RuleRateAtReportDate   ConversionRule = "report_date"
)

func IsConversionRule(r ConversionRule) bool {
	switch r {
	case RuleRateAtPostingDate, RuleRateOfPostingMonth, RuleAverageOverPeriod, RuleRateAtReportDate:
		return true
	}
	return false
}

type CurrencyAmount struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

type MissingRate struct {
	From string `json:"from"`
	To   string `json:"to"`
	On   string `json:"on"`
}

type UsedRate struct {
	From   string `json:"from"`
	To     string `json:"to"`
	On     string `json:"on"`
	Rate   int64  `json:"rate"`
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
	RateID string `json:"rate_id,omitempty"`
}

type ConvertedLine struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Department string           `json:"department"`
	AccountID  string           `json:"account_id"`
	Currency   string           `json:"currency"`
	Planned    int64            `json:"planned"`
	Spent      int64            `json:"spent"`
	Left       int64            `json:"left"`
	Original   []CurrencyAmount `json:"original"`
	Missing    []MissingRate    `json:"missing,omitempty"`
}

type ExchangeDifference struct {
	BillID     string `json:"bill_id"`
	PaymentID  string `json:"payment_id"`
	Currency   string `json:"currency"`
	Paid       int64  `json:"paid"`
	AtBilling  int64  `json:"at_billing"`
	AtPayment  int64  `json:"at_payment"`
	Difference int64  `json:"difference"`
	Outcome    string `json:"outcome"`
}

type ConvertedReport struct {
	PeriodStart         string               `json:"period_start"`
	PeriodEnd           string               `json:"period_end"`
	ReportingCurrency   string               `json:"reporting_currency"`
	Rule                ConversionRule       `json:"rule"`
	Planned             int64                `json:"planned"`
	Spent               int64                `json:"spent"`
	Difference          int64                `json:"difference"`
	Complete            bool                 `json:"complete"`
	Original            []CurrencyAmount     `json:"original"`
	Lines               []ConvertedLine      `json:"lines"`
	Rates               []UsedRate           `json:"rates"`
	Missing             []MissingRate        `json:"missing,omitempty"`
	ExchangeDifferences []ExchangeDifference `json:"exchange_differences"`
}

type ConvertedReportInput struct {
	PeriodStart       string         `json:"period_start"`
	PeriodEnd         string         `json:"period_end"`
	ReportingCurrency string         `json:"reporting_currency"`
	Rule              ConversionRule `json:"rule"`
}
