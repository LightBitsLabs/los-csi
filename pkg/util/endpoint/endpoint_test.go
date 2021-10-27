// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package endpoint_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

var nilEP = endpoint.EP{}

func TestStrictness(t *testing.T) {
	type testCase struct {
		tgt      string
		ep       endpoint.EP
		strictEP endpoint.EP
	}
	epA80 := endpoint.MustParse("a:80")
	tcs := []testCase{
		{"", nilEP, nilEP},
		{" ", nilEP, nilEP},
		{"a", nilEP, nilEP},
		{"a:", nilEP, nilEP},
		{"a:b", nilEP, nilEP},
		{":80", nilEP, nilEP},
		{"a:80", epA80, epA80},
		{"a: 80", nilEP, nilEP},
		{"a :80", nilEP, nilEP},
		{" a:80", epA80, nilEP},
		{"a:80 ", epA80, nilEP},
		{" a:80 ", epA80, nilEP},
		{" a :80", nilEP, nilEP},
		{"a :80 ", nilEP, nilEP},
		{" a :80 ", nilEP, nilEP},
		{" a b:80", nilEP, nilEP},
		{"a b:80 ", nilEP, nilEP},
		{" a b:80 ", nilEP, nilEP},

		{"user#host.com:80", nilEP, nilEP},
		{"host_name:80", nilEP, nilEP},
		{"host_name:80", nilEP, nilEP},
		{"host-name\\:80", nilEP, nilEP},
		{"host^name:80", nilEP, nilEP},
		{"host!name:80", nilEP, nilEP},
		{"host&name:80", nilEP, nilEP},

		{"user@host.com", nilEP, nilEP},
		{"user:pass@host.com", nilEP, nilEP},
		{"user:pass@host.com:80", nilEP, nilEP},
		{"user:pass@host.com:80/path", nilEP, nilEP},
		{"user:pass@host.com/path", nilEP, nilEP},
		{"user@host.com:80/path", nilEP, nilEP},
		{"host.com:80/path", nilEP, nilEP},
		{"user@host.com/path", nilEP, nilEP},
		{"1.1.1.1:80000", nilEP, nilEP},
		{"2001:0db8:0a0b:12f0:0000:0000:0000:0001:443", nilEP, nilEP},
	}

	chkRes := func(
		t *testing.T, name string, ep endpoint.EP, err error, tgt string, exp endpoint.EP,
	) {
		if err != nil {
			if exp != nilEP {
				t.Errorf("BUG: %s(%s) failed instead of '%s'",
					name, tgt, exp)
			} else if testing.Verbose() {
				t.Logf("OK: %s(%s) refused: %s", name, tgt, err)
			}
		} else if err == nil && exp == nilEP {
			t.Errorf("BUG: %s(%s) succeeded: '%s'", name, tgt, ep)
		} else if ep != exp {
			t.Errorf("BUG: %s(%s) => bad result:\nEXP:%v\nGOT:%v",
				name, tgt, exp, ep)
		} else if testing.Verbose() {
			t.Logf("OK: %s(%s) => '%s'", name, tgt, ep)
		}
	}

	for _, tc := range tcs {
		t.Run(tc.tgt, func(t *testing.T) {
			ep, err := endpoint.Parse(tc.tgt)
			chkRes(t, "Parse", ep, err, tc.tgt, tc.ep)
			sep, err := endpoint.ParseStricter(tc.tgt)
			chkRes(t, "ParseStricter", sep, err, tc.tgt, tc.strictEP)
		})
	}
}

func TestParseSliceCSV(t *testing.T) {
	type testCase struct {
		tgts  []string
		eps   endpoint.Slice
		csvOK bool
	}

	tgtA80 := "a:80"
	epA80 := endpoint.MustParse(tgtA80)
	tgtA81 := "a:81"
	epA81 := endpoint.MustParse(tgtA81)
	tgtB80 := "b:80"
	epB80 := endpoint.MustParse(tgtB80)
	tgtEx := "nvme-of.example.com:4420"
	epEx := endpoint.MustParse(tgtEx)
	tgt192a := "192.168.0.1:8080"
	ep192a := endpoint.MustParse(tgt192a)
	tgt192b := "192.168.0.1:80"
	ep192b := endpoint.MustParse(tgt192b)
	tgtFoo := "foo.bar.baz:80"
	epFoo := endpoint.MustParse(tgtFoo)
	tgt6a := "[2001:0db8:0a0b:12f0:0000:0000:0000:0001]:443"
	ep6a := endpoint.MustParse(tgt6a)
	tgt6b := "[2001:db8::1]:80"
	ep6b := endpoint.MustParse(tgt6b)
	tgt6z := "[fe80::1%zone]:80"
	ep6z := endpoint.MustParse(tgt6z)

	tcs := []testCase{
		{[]string{""}, nil, false},
		{[]string{"", ""}, nil, false},
		{[]string{tgtA80, ""}, nil, false},
		{[]string{tgtA80, " "}, nil, false},
		{[]string{tgtA80, " ", tgtB80}, nil, false},
		{[]string{tgtA80, "", tgtB80}, nil, false},
		{[]string{"a"}, nil, false},
		{[]string{"a:"}, nil, false},
		{[]string{":80"}, nil, false},
		{[]string{" :80"}, nil, false},
		{[]string{",a:80", tgtB80}, nil, false},
		{[]string{"a:80,", tgtB80}, nil, false},
		{[]string{tgtA80, "b:80,"}, nil, false},
		{[]string{tgtA80, ",b:80"}, nil, false},
		{[]string{tgtA80, "a", tgtB80}, nil, false},
		{
			[]string{tgtA80},
			[]endpoint.EP{epA80},
			true,
		},
		{
			[]string{" a:80", tgtB80},
			[]endpoint.EP{epA80, epB80},
			false,
		},
		{
			[]string{tgtA80, "b:80 "},
			[]endpoint.EP{epA80, epB80},
			false,
		},
		{
			[]string{"a:80 ", tgtB80},
			[]endpoint.EP{epA80, epB80},
			false,
		},
		{
			[]string{tgtA80, " b:80"},
			[]endpoint.EP{epA80, epB80},
			false,
		},
		{
			[]string{tgt6a, tgt6z, tgt6b},
			[]endpoint.EP{ep6a, ep6b, ep6z},
			true,
		},
		{
			[]string{" a:80", " a:80 ", "a:80 "},
			[]endpoint.EP{epA80},
			false,
		},
		{
			[]string{" a:80", "a:80", "a:80 "},
			[]endpoint.EP{epA80},
			false,
		},
		{
			[]string{tgtA80, tgtB80},
			[]endpoint.EP{epA80, epB80},
			true,
		},
		{
			[]string{tgtB80, tgtA80},
			[]endpoint.EP{epA80, epB80},
			true,
		},
		{
			[]string{tgtA80, tgtA81},
			[]endpoint.EP{epA80, epA81},
			true,
		},
		{
			[]string{tgtA81, tgtA80},
			[]endpoint.EP{epA80, epA81},
			true,
		},
		{
			[]string{tgtA80, tgtA80},
			[]endpoint.EP{epA80},
			true,
		},
		{
			[]string{tgtA80, tgtA81, tgtA80},
			[]endpoint.EP{epA80, epA81},
			true,
		},
		{
			[]string{tgtB80, tgtA80, tgtA80, tgtA81, tgtB80, tgtA80},
			[]endpoint.EP{epA80, epA81, epB80},
			true,
		},
		{
			[]string{tgt192a, tgt192b, tgtFoo, tgt192a, tgt192b},
			[]endpoint.EP{ep192b, ep192a, epFoo},
			true,
		},
		{
			[]string{tgtA80, tgt192a, tgt192b, tgtEx, tgt6b},
			[]endpoint.EP{ep192b, ep192a, ep6b, epA80, epEx},
			true,
		},
	}

	chkRes := func(
		t *testing.T, name string, eps endpoint.Slice, err error, tgts string,
		exp endpoint.Slice,
	) {
		if err != nil {
			if exp != nil {
				t.Errorf("BUG: %s(%s) failed", name, tgts)
			} else if testing.Verbose() {
				t.Logf("OK: %s(%s) refused: %s", name, tgts, err)
			}
		} else if err == nil && exp == nil {
			t.Errorf("BUG: %s(%s) succeeded: [%s]", name, tgts, eps)
		} else if !eps.Equal(exp) {
			t.Errorf("BUG: %s(%s) => bad result:\nEXP:%v\nGOT:%v",
				name, tgts, exp, eps)
		} else if testing.Verbose() {
			t.Logf("OK: %s(%s) => [%s]", name, tgts, eps)
		}
	}

	for _, tc := range tcs {
		targets := strings.Join(tc.tgts, ",")
		t.Run(targets, func(t *testing.T) {
			eps, err := endpoint.ParseSlice(tc.tgts)
			chkRes(t, "ParseSlice", eps, err, fmt.Sprintf("%q", tc.tgts), tc.eps)

			eps2, err := endpoint.ParseCSV(targets)
			var exp endpoint.Slice
			if tc.csvOK {
				exp = tc.eps
			}
			chkRes(t, "ParseCSV", eps2, err, targets, exp)
		})
	}
}

func TestParseSliceIPAddys(t *testing.T) {
	type testCase struct {
		tgts []string
		eps  endpoint.Slice
		eps4 endpoint.Slice
	}

	tgt4a := "192.168.0.1:80"
	ep4a := endpoint.MustParse(tgt4a)
	tgt4b := "1.1.1.1:8080"
	ep4b := endpoint.MustParse(tgt4b)
	tgtN1 := "1.2.3:80"
	epN1 := endpoint.MustParse(tgtN1)
	tgtN2 := "1.2.3.4.5:80"
	epN2 := endpoint.MustParse(tgtN2)
	tgtFoo := "foo.bar.baz:80"
	epFoo := endpoint.MustParse(tgtFoo)
	tgt6a := "[2001:0db8:0a0b:12f0:0000:0000:0000:0001]:443"
	ep6a := endpoint.MustParse(tgt6a)

	tcs := []testCase{
		{[]string{}, endpoint.Slice{}, endpoint.Slice{}},
		{[]string{""}, nil, nil},
		{[]string{"1.2.3.4"}, nil, nil},
		{
			[]string{tgt4a},
			[]endpoint.EP{ep4a},
			[]endpoint.EP{ep4a},
		},
		{
			[]string{tgt4b, tgt4a},
			[]endpoint.EP{ep4b, ep4a},
			[]endpoint.EP{ep4b, ep4a},
		},
		{
			[]string{tgt4a, tgt4a},
			[]endpoint.EP{ep4a},
			[]endpoint.EP{ep4a},
		},
		{
			[]string{tgtFoo},
			[]endpoint.EP{epFoo},
			nil,
		},
		{
			[]string{tgtFoo, tgt4a},
			[]endpoint.EP{ep4a, epFoo},
			nil,
		},
		{
			[]string{tgt4a, tgt6a, tgt4b},
			[]endpoint.EP{ep4b, ep4a, ep6a},
			nil,
		},
		{
			[]string{tgt4b, tgtN1},
			[]endpoint.EP{ep4b, epN1},
			nil,
		},
		{
			[]string{tgtN2, tgt4a},
			[]endpoint.EP{epN2, ep4a},
			nil,
		},
	}

	chkRes := func(
		t *testing.T, name string, eps endpoint.Slice, err error, tgts string,
		exp endpoint.Slice,
	) {
		if err != nil {
			if exp != nil {
				t.Errorf("BUG: %s(%s) failed", name, tgts)
			} else if testing.Verbose() {
				t.Logf("OK: %s(%s) refused: %s", name, tgts, err)
			}
		} else if err == nil && exp == nil {
			t.Errorf("BUG: %s(%s) succeeded: [%s]", name, tgts, eps)
		} else if !eps.Equal(exp) {
			t.Errorf("BUG: %s(%s) => bad result:\nEXP:%v\nGOT:%v",
				name, tgts, exp, eps)
		} else if testing.Verbose() {
			t.Logf("OK: %s(%s) => [%s]", name, tgts, eps)
		}
	}

	for _, tc := range tcs {
		targets := strings.Join(tc.tgts, ",")
		t.Run(targets, func(t *testing.T) {
			eps, err := endpoint.ParseSlice(tc.tgts)
			chkRes(t, "ParseSlice", eps, err, targets, tc.eps)
			eps4, err := endpoint.ParseSliceIPv4(tc.tgts)
			chkRes(t, "ParseSliceIPv4", eps4, err, targets, tc.eps4)
		})
	}
}
