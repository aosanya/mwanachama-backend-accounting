package accounting

import _ "embed"

//go:embed accounting.operations.json
var operationsJSON []byte

func Operations() []byte { return operationsJSON }
