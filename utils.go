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

	"github.com/urfave/cli"
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

func unsupportedKind(k reflect.Kind) error {
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
	if v.CanInterface() && v.CanAddr() {
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
	return nil, unsupportedKind(k)
}

func setPrimitiveValueFromString(v reflect.Value, arg string) error {
	if v.CanInterface() && v.CanAddr() {
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
		return unsupportedKind(k)
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
