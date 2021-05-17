// Copyright (C) 2019 Audrius Butkevicius
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package recli

import (
	"encoding"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"

	"github.com/pkg/errors"

	"github.com/urfave/cli"
)

type ValuePrinter func(interface{})
type KeyValuePrinter func(interface{}, interface{})
type FieldNameConverter func(string) string

type Tag struct {
	Name  string
	Value string
}

type Config struct {
	SkipTag            Tag
	IDTag              Tag
	UsageTagName       string
	DefaultTagName     string
	FieldNameConverter FieldNameConverter
	ValuePrinter       ValuePrinter
	KeyValuePrinter    KeyValuePrinter
}

var (
	DefaultConfig = Config{
		SkipTag: Tag{
			Name:  "recli",
			Value: "-",
		},
		IDTag: Tag{
			Name:  "recli",
			Value: "id",
		},
		UsageTagName:       "usage",
		DefaultTagName:     "default",
		FieldNameConverter: toLowerDashCase,
		ValuePrinter: func(value interface{}) {
			fmt.Println(value)
		},
		KeyValuePrinter: func(key interface{}, value interface{}) {
			fmt.Println(key, " = ", value)
		},
	}
	Default = New(DefaultConfig)
)

func New(config Config) Constructor {
	return &constructor{
		cfg: config,
	}
}

type Constructor interface {
	Construct(item interface{}) ([]cli.Command, error)
}

type constructor struct {
	cfg Config
}

func (c *constructor) printValue(v reflect.Value) error {
	val, err := getPrimitiveValue(v)
	if err != nil {
		return err
	}
	c.cfg.ValuePrinter(val)
	return nil
}

func (c *constructor) makePrimitiveCommands(v reflect.Value) []cli.Command {
	cmds := []cli.Command{
		{
			Name:     "get",
			Usage:    "Get the value",
			Category: "ACTIONS",
			Action: expectArgs(0, func(ctx *cli.Context) error {
				return c.printValue(v)
			}),
		},
	}

	if v.CanSet() {
		cmds = append(cmds, cli.Command{
			Name:      "set",
			ArgsUsage: "[value]",
			Usage:     "Set the value",
			Category:  "ACTIONS",
			Action: expectArgs(1, func(ctx *cli.Context) error {
				return setPrimitiveValueFromString(v, ctx.Args().First())
			}),
		})
	}
	return cmds
}

func (c *constructor) makeMapCommands(v reflect.Value) []cli.Command {
	return []cli.Command{
		{
			Name:     "dump",
			Usage:    "Dump all keys and their values",
			Category: "ACTIONS",
			Action: expectArgs(0, func(ctx *cli.Context) error {
				for _, keyValue := range v.MapKeys() {
					valueValue := v.MapIndex(keyValue)
					keyInterface, err := getPrimitiveValue(keyValue)
					if err != nil {
						return err
					}
					valueInterface, err := getPrimitiveValue(valueValue)
					if err != nil {
						return err
					}
					c.cfg.KeyValuePrinter(keyInterface, valueInterface)
				}
				return nil
			}),
		},
		{
			Name:      "get",
			ArgsUsage: "[key]",
			Usage:     "Get the value of a given key",
			Category:  "ACTIONS",
			Action: expectArgs(1, func(ctx *cli.Context) error {
				keyValue, err := stringToPrimitiveValue(ctx.Args().First(), v.Type().Key())
				if err != nil {
					return err
				}
				valueValue := v.MapIndex(keyValue)
				return c.printValue(valueValue)
			}),
		},
		{
			Name:      "set",
			ArgsUsage: "[key] [value]",
			Usage:     "Set the key to the given value",
			Category:  "ACTIONS",
			Action: expectArgs(2, func(ctx *cli.Context) error {
				keyValue, err := stringToPrimitiveValue(ctx.Args().First(), v.Type().Key())
				if err != nil {
					return err
				}
				valueValue, err := stringToPrimitiveValue(ctx.Args().Get(1), v.Type().Elem())
				if err != nil {
					return err
				}
				v.SetMapIndex(keyValue, valueValue)
				return nil
			}),
		},
		{
			Name:      "unset",
			ArgsUsage: "[key]",
			Usage:     "Remove the key from the map",
			Category:  "ACTIONS",
			Action: expectArgs(1, func(ctx *cli.Context) error {
				keyValue, err := stringToPrimitiveValue(ctx.Args().First(), v.Type().Key())
				if err != nil {
					return err
				}
				v.SetMapIndex(keyValue, reflect.Value{})
				return nil
			}),
		},
	}
}

func makeJsonDumper(v reflect.Value, printer func(string)) cli.Command {
	return cli.Command{
		Name:     "dump-json",
		Usage:    "Dump item as json",
		Category: "ACTIONS",
		Action: expectArgs(0, func(ctx *cli.Context) error {
			var vi interface{}
			if v.CanAddr() && v.Addr().CanInterface() {
				vi = v.Addr().Interface()
			} else {
				return fmt.Errorf("Cannot dump %s as json", v.Type())
			}
			bytes, err := json.MarshalIndent(vi, "", "  ")
			if err != nil {
				return err
			}
			printer(string(bytes))
			return nil
		}),
	}
}

func (c *constructor) makeSliceAccessorCommands(keyer func(int) (string, error), v reflect.Value) ([]cli.Command, error) {
	cmds := make([]cli.Command, 0, v.Len())
	for vi := 0; vi < v.Len(); vi++ {
		idx := vi // Copy loop variable
		key, err := keyer(idx)
		if err != nil {
			return nil, err
		}
		keyCmds, err := c.getCommandsForValue(v.Index(idx))
		keyCmds = append(keyCmds, cli.Command{
			Name:     "delete",
			Usage:    fmt.Sprintf("Delete item represented by key %q from the collection", key),
			Category: "ACTIONS",
			Action: expectArgs(0, func(ctx *cli.Context) error {
				v.Set(reflect.AppendSlice(v.Slice(0, idx), v.Slice(idx+1, v.Len())))
				return nil
			}),
		})
		cmds = append(cmds, cli.Command{
			Name:        key,
			Category:    "ITEMS",
			Subcommands: keyCmds,
		})
	}
	return cmds, nil
}

func (c *constructor) makeSliceCommands(v reflect.Value) ([]cli.Command, error) {
	member := v.Type().Elem()

	keyer := func(i int) (string, error) {
		return fmt.Sprint(i), nil
	}

	primitive := isPrimitiveKind(member.Kind())

	if !primitive && member.Kind() != reflect.Struct {
		return nil, unsupportedKindErr(member.Kind())
	}

	if !primitive {
		for mi := 0; mi < member.NumField(); mi++ {
			if hasTag(member.Field(mi), c.cfg.IDTag) {
				keyer = func(i int) (string, error) {
					val, err := getPrimitiveValue(v.Index(i).Field(mi))
					return fmt.Sprint(val), err
				}
				break
			}
		}
	}

	cmds := make([]cli.Command, 0, v.Len()+2)
	if accessCmds, err := c.makeSliceAccessorCommands(keyer, v); err != nil {
		return nil, err
	} else {
		cmds = append(cmds, accessCmds...)
	}

	cmds = append(cmds, cli.Command{
		Name:     "list",
		Usage:    "List item keys in the collection",
		Category: "ACTIONS",
		Action: expectArgs(0, func(ctx *cli.Context) error {
			for vi := 0; vi < v.Len(); vi++ {
				idx := vi // Copy loop variable
				key, err := keyer(idx)
				if err != nil {
					return err
				}
				c.cfg.ValuePrinter(key)
			}
			return nil
		}),
	})

	if primitive {
		cmds = append(cmds, cli.Command{
			Name:      "add",
			Usage:     "Add a new item to collection",
			ArgsUsage: "[value]",
			Category:  "ACTIONS",
			Action: expectArgs(1, func(ctx *cli.Context) error {
				newValue, err := stringToPrimitiveValue(ctx.Args().First(), member)
				if err != nil {
					return err
				}
				v.Set(reflect.Append(v, newValue))
				return nil
			}),
		})
	} else {
		cmds = append(cmds, c.makeSliceItemBuilders(v)...)
	}

	return cmds, nil
}

func (c *constructor) makeSliceItemBuilderFlags(memberType reflect.Type) []cli.Flag {
	flags := make([]cli.Flag, 0, memberType.NumField())
	for fi := 0; fi < memberType.NumField(); fi++ {
		memberField := memberType.Field(fi)
		usage := ""
		if defaultValueString, ok := memberField.Tag.Lookup(c.cfg.DefaultTagName); ok {
			usage = fmt.Sprintf("default value: %s", defaultValueString)
		}

		memberKind := simplifyKind(memberField.Type.Kind())
		memberKindIsTextUnmarshaler := memberField.Type.Implements(textUnmarshaler) || reflect.PtrTo(memberField.Type).Implements(textUnmarshaler)

		switch {
		case memberKind == reflect.Bool:
			flags = append(flags, cli.BoolFlag{
				Name:  c.cfg.FieldNameConverter(memberField.Name),
				Usage: usage,
			})
		case memberKind == reflect.String || memberKindIsTextUnmarshaler:
			flags = append(flags, cli.StringFlag{
				Name:  c.cfg.FieldNameConverter(memberField.Name),
				Usage: usage,
			})
		case memberKind == reflect.Int:
			flags = append(flags, cli.Int64Flag{
				Name:  c.cfg.FieldNameConverter(memberField.Name),
				Usage: usage,
			})
		case memberKind == reflect.Float32 || memberKind == reflect.Float64:
			flags = append(flags, cli.Float64Flag{
				Name:  c.cfg.FieldNameConverter(memberField.Name),
				Usage: usage,
			})
		case memberKind == reflect.Array || memberKind == reflect.Slice:
			arrayKind := simplifyKind(memberField.Type.Elem().Kind())
			elemType := memberField.Type.Elem()
			arrayKindIsTextUnmarshaler := elemType.Implements(textUnmarshaler) || reflect.PtrTo(elemType).Implements(textUnmarshaler)
			switch {
			case arrayKind == reflect.Int:
				flags = append(flags, cli.Int64SliceFlag{
					Name: c.cfg.FieldNameConverter(memberField.Name),
				})
			case arrayKind == reflect.String || arrayKindIsTextUnmarshaler:
				flags = append(flags, cli.StringSliceFlag{
					Name: c.cfg.FieldNameConverter(memberField.Name),
				})
			}
		}
	}
	return flags
}

func (c *constructor) makeSliceItemBuilders(v reflect.Value) []cli.Command {
	memberType := v.Type().Elem()

	return []cli.Command{
		{
			Name:      "add",
			Usage:     "Add a new item to collection",
			ArgsUsage: "-attribute=value",
			Category:  "ACTIONS",
			Flags:     c.makeSliceItemBuilderFlags(memberType),
			Action: expectArgs(0, func(ctx *cli.Context) error {
				if ctx.NumFlags() == 0 {
					return errors.New("no properties specified")
				}

				// Create a new item that will go in the slice
				newValue := reflect.New(memberType).Elem()

				// Set defaults
				if err := setDefaults(c.cfg.DefaultTagName, newValue.Addr().Interface(), nil); err != nil {
					return err
				}

				for mi := 0; mi < newValue.NumField(); mi++ {
					flagName := c.cfg.FieldNameConverter(memberType.Field(mi).Name)
					fieldValue := deref(newValue.Field(mi))
					if ctx.IsSet(flagName) {
						if isPrimitive(fieldValue) {
							if err := setPrimitiveValueFromString(fieldValue, ctx.Generic(flagName).(flag.Value).String()); err != nil {
								return err
							}
							continue
						}

						switch fieldValue.Kind() {
						case reflect.Array, reflect.Slice:
							switch simplifyKind(fieldValue.Elem().Kind()) {
							case reflect.Int:
								fieldValue.Set(reflect.ValueOf(ctx.IntSlice(flagName)))
							case reflect.String:
								fieldValue.Set(reflect.ValueOf(ctx.StringSlice(flagName)))
							}
						}
					}
				}
				v.Set(reflect.Append(v, newValue))
				return nil
			}),
		},
		{
			Name:      "add-json",
			Usage:     "Add a new item to collection deserialised from JSON",
			ArgsUsage: "[value]",
			Category:  "ACTIONS",
			Action: expectArgs(1, func(ctx *cli.Context) error {
				newValue := reflect.New(memberType)
				if err := json.Unmarshal([]byte(ctx.Args().First()), newValue.Interface()); err != nil {
					return err
				}
				v.Set(reflect.Append(v, newValue.Elem()))
				return nil
			}),
		},
	}
}

func (c *constructor) Construct(item interface{}) ([]cli.Command, error) {
	itemValue := reflect.ValueOf(item)
	if itemValue.Kind() != reflect.Ptr {
		return nil, errors.New("expected a pointer got: " + itemValue.Kind().String())
	}
	itemValue = itemValue.Elem()
	if itemValue.Kind() != reflect.Struct {
		return nil, errors.New("expected pointer to a struct got a pointer to: " + itemValue.Kind().String())
	}
	itemType := itemValue.Type()

	cmds := make([]cli.Command, 0, itemType.NumField())
	for i := 0; i < itemType.NumField(); i++ {
		f := itemType.Field(i)
		v := itemValue.Field(i)

		// This is what encoding/json does
		isUnexported := f.PkgPath != ""
		if f.Anonymous || hasTag(f, c.cfg.SkipTag) || isUnexported {
			continue
		}

		valueCmds, err := c.getCommandsForValue(v)
		if err != nil {
			return nil, errors.Wrap(err, f.Name)
		}
		cmds = append(cmds, cli.Command{
			Name:        c.cfg.FieldNameConverter(f.Name),
			Usage:       f.Tag.Get(c.cfg.UsageTagName),
			Category:    "PROPERTIES",
			Subcommands: valueCmds,
		})
	}
	cmds = append(cmds, makeJsonDumper(itemValue, func(s string) {
		c.cfg.ValuePrinter(s)
	}))

	return cmds, nil
}

func isPrimitiveKind(k reflect.Kind) bool {
	return (reflect.Bool <= k && k <= reflect.Float64) || k == reflect.String
}

func isPrimitive(v reflect.Value) bool {
	v = deref(v)

	k := v.Kind()
	if isPrimitiveKind(k) {
		return true
	}

	if v.CanAddr() && v.Addr().CanInterface() {
		vi := v.Addr().Interface()
		_, okm := vi.(encoding.TextMarshaler)
		_, oku := vi.(encoding.TextUnmarshaler)
		if okm && oku {
			return true
		}
	}
	return false
}

func (c *constructor) getCommandsForValue(v reflect.Value) ([]cli.Command, error) {
	v = deref(v)
	k := v.Kind()

	switch {
	case isPrimitive(v):
		return c.makePrimitiveCommands(v), nil

	case k == reflect.Map:
		return c.makeMapCommands(v), nil

	case k == reflect.Struct && v.CanAddr() && v.Addr().CanInterface():
		return c.Construct(v.Addr().Interface())

	case k == reflect.Slice || k == reflect.Array:
		return c.makeSliceCommands(v)
	}

	return nil, unsupportedKindErr(k)
}
