package routes

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"
	"github.com/aosanya/mwanachama-backend-shared/httpwire"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

type Route = httpwire.Route

var operations = sync.OnceValues(func() (*dispatch.Spec, error) {
	return dispatch.Parse(accounting.Operations())
})

var sentinels = map[string]error{
	"ErrNotFound":        accounting.ErrNotFound,
	"ErrInvalid":         accounting.ErrInvalid,
	"ErrClosed":          accounting.ErrClosed,
	"ErrNonZeroBalance":  accounting.ErrNonZeroBalance,
	"ErrAlreadyReversed": accounting.ErrAlreadyReversed,
	"ErrCodeTaken":       accounting.ErrCodeTaken,
}

type Mount struct {
	Authorize dispatch.Authorizer
	Caller    dispatch.Caller
}

func Build(repo accounting.LedgerRepository) ([]Route, error) { return BuildWith(repo, nil) }

func BuildWith(repo accounting.LedgerRepository, authorize dispatch.Authorizer) ([]Route, error) {
	return BuildFor(repo, Mount{Authorize: authorize})
}

func BuildFor(repo accounting.LedgerRepository, m Mount) ([]Route, error) {
	s, err := operations()
	if err != nil {
		return nil, err
	}
	return dispatch.Dispatch(s, dispatch.Deps{
		Manager: repo, Errors: sentinels, Authorize: m.Authorize, Caller: m.Caller,
	})
}

func Routes(repo accounting.LedgerRepository) []Route { return RoutesWith(repo, nil) }

func RoutesWith(repo accounting.LedgerRepository, authorize dispatch.Authorizer) []Route {
	return RoutesFor(repo, Mount{Authorize: authorize})
}

func RoutesFor(repo accounting.LedgerRepository, m Mount) []Route {
	out, err := BuildFor(repo, m)
	if err != nil {
		panic(fmt.Sprintf("accounting routes: %v", err))
	}
	return out
}

func Shape() []Route {
	s, err := operations()
	if err != nil {
		panic(fmt.Sprintf("accounting routes: %v", err))
	}
	names := make([]string, 0, len(s.Operations))
	for name := range s.Operations {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Route, 0, len(names))
	for _, name := range names {
		op := s.Operations[name]
		out = append(out, Route{Method: op.Method, Path: s.Base + op.Path, Action: op.Action})
	}
	return out
}
