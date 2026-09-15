package accounting

// AccountType is the account's place in the accounting equation — the one
// vocabulary this package closes, because it is double-entry theory itself,
// not a decision any particular domain gets to make differently.
type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeEquity    AccountType = "equity"
	AccountTypeIncome    AccountType = "income"
	AccountTypeExpense   AccountType = "expense"
)

var accountTypes = map[AccountType]bool{
	AccountTypeAsset: true, AccountTypeLiability: true, AccountTypeEquity: true,
	AccountTypeIncome: true, AccountTypeExpense: true,
}

// IsAccountType reports whether t is one of the five standard types.
func IsAccountType(t AccountType) bool { return accountTypes[t] }

// AccountKind groups accounts for a caller's own purposes — "a member's
// account", "a paybill's basket", "a school's fee account", "a chain's
// treasury wallet". This package does not know what any value means or
// validate it against a fixed list: unlike AccountType, what kinds of
// account a deployment has is exactly the part that differs between a
// political membership org, a school, and a token ledger, so it stays a
// plain caller-defined string, the same discipline
// mwanachama-backend-assetmanager's Location.Kind already applies to
// "warehouse"/"aisle"/"pasture". A domain layered on top of this package
// (e.g. a contribution subpackage) is where fixed AccountKind values and
// their conventions belong — see that layer's own vocabulary, not this
// package's.
type AccountKind string

// DocumentKind names what caused a posting, in whatever vocabulary the
// calling domain uses ("contribution", "tuition_payment",
// "token_transfer"...). Open for the same reason AccountKind is: this
// package enforces that every entry names one, never which ones exist.
type DocumentKind string
