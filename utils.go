// Copyright (C) 2019 Audrius Butkevicius
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package recli

import (
	"encoding"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/pkg/errors"

	"github.com/urfave/cli"
)

type ParseDefaulter interface {
	ParseDefault(string) error
}

var (
	textUnmarshaler = reflect.TypeOf(new(encoding.TextUnmarshaler)).Elem()
)

func hasTag(field reflect.StructField, tag Tag) bool {
	for _, existingValue := range strings.Split(field.Tag.Get(tag.Name), ",") {
		if tag.Value == existingValue {
			return true
		}
	}
	return false
}

func simplifyKind(k reflect.Kind) reflect.Kind {
	if reflect.Int <= k && k <= reflect.Uintptr {
		return reflect.Int
	}
	return k
}

func unsupportedKindErr(k reflect.Kind) error {
	_, fn, line, _ := runtime.Caller(1)
	fileParts := strings.Split(fn, "/")
	return fmt.Errorf("unsupported kind: %s [%s:%d]", k, fileParts[len(fileParts)-1], line)
}

func toLowerDashCase(arg string) string {
	output := make([]rune, 0, len(arg))
	previousUppercase := false
	for i, r := range arg {
		if i == 0 {
			output = append(output, unicode.ToLower(r))
		} else if unicode.IsUpper(r) {
			// If it's the last rune, and it's uppercase, it's probably a unit suffix, so skip the dash
			if !previousUppercase && i != len(arg)-1 {
				output = append(output, '-')
			}
			output = append(output, unicode.ToLower(r))
		} else {
			output = append(output, r)
		}
		previousUppercase = unicode.IsUpper(r)

	}
	return string(output)
}

func getPrimitiveValue(v reflect.Value) (interface{}, error) {
	// Always expect a non-pointer
	if v.CanAddr() && v.Addr().CanInterface() {
		if m, ok := v.Addr().Interface().(encoding.TextMarshaler); ok {
			v, err := m.MarshalText()
			return string(v), err
		}
	}

	k := simplifyKind(v.Kind())
	switch k {
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int:
		return v.Int(), nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	case reflect.String:
		return v.String(), nil
	}
	return nil, unsupportedKindErr(k)
}

func setPrimitiveValueFromString(v reflect.Value, arg string) error {
	// Always expect a non-pointer
	if v.CanAddr() && v.Addr().CanInterface() {
		if m, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return m.UnmarshalText([]byte(arg))
		}
	}

	k := simplifyKind(v.Kind())
	switch k {
	case reflect.Bool:
		if cv, err := strconv.ParseBool(arg); err != nil {
			return err
		} else {
			v.SetBool(cv)
		}

	case reflect.Int:
		if cv, err := strconv.ParseInt(arg, 0, 0); err != nil {
			return err
		} else if v.OverflowInt(cv) {
			return fmt.Errorf("value overflows: %d", cv)
		} else {
			v.SetInt(cv)
		}

	case reflect.Float32, reflect.Float64:
		if cv, err := strconv.ParseFloat(arg, 0); err != nil {
			return err
		} else {
			v.SetFloat(cv)
		}

	case reflect.String:
		v.SetString(arg)

	default:
		return unsupportedKindErr(k)
	}

	return nil
}

func stringToPrimitiveValue(arg string, t reflect.Type) (reflect.Value, error) {
	v := reflect.New(t).Elem()
	return v, setPrimitiveValueFromString(v, arg)
}

func expectArgs(n int, actionFunc cli.ActionFunc) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if ctx.NArg() != n {
			pluarl := ""
			if n != 1 {
				pluarl = "s"
			}
			return fmt.Errorf("expected %d arugment%s, got %d", n, pluarl, ctx.NArg())
		}
		return actionFunc(ctx)
	}
}

func setDefaults(tagName string, data interface{}, seen map[uintptr]struct{}) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	if seen == nil {
		seen = make(map[uintptr]struct{})
	} else if s.CanAddr() {
		if _, ok := seen[s.Addr().Pointer()]; ok {
			return nil
		}
	}

	seen[s.Addr().Pointer()] = struct{}{}

	for i := 0; i < s.NumField(); i++ {
		f := deref(s.Field(i))
		tag := t.Field(i).Tag

		if f.Kind() == reflect.Struct {
			if f.CanAddr() && f.Addr().CanInterface() {
				err := setDefaults(tagName, f.Addr().Interface(), seen)
				if err != nil {
					return err
				}
				continue
			}
		}

		v := tag.Get(tagName)
		if len(v) == 0 {
			continue
		}

		if f.CanAddr() && f.Addr().CanInterface() {
			if i, ok := f.Addr().Interface().(ParseDefaulter); ok {
				if err := i.ParseDefault(v); err != nil {
					return err
				}
				continue
			}
		}

		if isPrimitive(f) {
			err := setPrimitiveValueFromString(f, v)
			if err != nil {
				return err
			}
			continue
		}

		switch f.Kind() {
		case reflect.Array, reflect.Slice:
			switch simplifyKind(f.Type().Elem().Kind()) {
			case reflect.Int:
				var m []int
				for _, si := range strings.Split(v, ",") {
					i, err := strconv.ParseInt(si, 10, 64)
					if err != nil {
						return err
					}
					m = append(m, int(i))
				}
				f.Set(reflect.ValueOf(m))
				continue
			case reflect.String:
				var m []string
				for _, i := range strings.Split(v, ",") {
					m = append(m, i)
				}
				f.Set(reflect.ValueOf(m))
				continue
			}
		}

		return errors.Wrap(unsupportedKindErr(f.Kind()), "setDefaults")
	}

	return nil
}

func deref(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v
}
