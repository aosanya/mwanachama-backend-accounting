package models

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

func IsAccountType(t AccountType) bool { return accountTypes[t] }

func AccountTypes() []AccountType {
	return []AccountType{
		AccountTypeAsset, AccountTypeLiability, AccountTypeEquity,
		AccountTypeIncome, AccountTypeExpense,
	}
}

type AccountCategory string

type DocumentKind string
