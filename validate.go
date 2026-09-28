package accounting

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

const (
	RoleAccount = roleAccount
	RoleEntry   = roleEntry
)

func Check(s *spec.Spec, role string, v any) error {
	o, ok := s.ByRole(role)
	if !ok {
		return fmt.Errorf("check: this domain fills no object for the role %q", role)
	}
	return check(o, v)
}

func check(o spec.Object, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("check %s: want a struct, got %T", o.Name, v)
	}

	byColumn := map[string]reflect.Value{}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		if rt.Field(i).PkgPath != "" {
			continue
		}
		byColumn[columnName(rt.Field(i).Name)] = rv.Field(i)
	}

	for _, f := range o.Fields {
		fv, ok := byColumn[f.Name]
		if !ok || fv.Kind() != reflect.String {
			continue
		}
		s := fv.String()

		if f.Required && strings.TrimSpace(s) == "" {
			return fmt.Errorf("%w: %s names no %s", ErrInvalid, o.Name, f.Name)
		}
		if f.Type == spec.TypeEnum && !declares(f.Values, s) {
			return fmt.Errorf("%w: %s is %q, which is not one of %s",
				ErrInvalid, f.Name, s, strings.Join(f.Values, ", "))
		}
		if f.Matches != "" && s != "" {
			ok, known := patterns[f.Matches]
			if !known {
				return fmt.Errorf("%w: %s names the pattern %q, which this module does not supply",
					ErrInvalid, f.Name, f.Matches)
			}
			if !ok(s) {
				return fmt.Errorf("%w: %s does not match %s", ErrInvalid, f.Name, f.Matches)
			}
		}
	}
	return nil
}

func declares(values []string, s string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}

func (m *ledgerStore) checks(role string, v any) error {
	return check(m.st.Object(role), v)
}
