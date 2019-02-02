// Copyright (C) 2019 Audrius Butkevicius
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package recli

import (
	"testing"
)

type Inner struct {
	A string `default:"inner"`
	B *DefaultStruct
}

type DefaultStruct struct {
	A string `default:"outer"`
	B []int  `default:"10,20"`
	C Inner
}

func TestSetDefault(t *testing.T) {
	x := &DefaultStruct{}
	x.C.B = x
	err := setDefaults("default", x, nil)
	if err != nil {
		t.Error(err)
	}
	if x.A != "outer" {
		t.Errorf("A")
	}
	if len(x.B) != 2 {
		t.Errorf("B %d", len(x.B))
	}
	if x.C.A != "inner" {
		t.Errorf("C.A")
	}
	if x.C.B.A != "outer" {
		t.Errorf("A loop")
	}
}
