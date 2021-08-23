// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var hostRegex *regexp.Regexp

func init() {
	hostRegex = regexp.MustCompile(`^([a-zA-Z0-9.\[\]:%-]+)$`)
}

type EP struct {
	host string // either a hostname or an IP address
	port uint16 // port number
}

func ParseStricter(endpoint string) (EP, error) {
	mkErr := func(format string, args ...interface{}) error {
		return fmt.Errorf("bad endpoint '%s': "+format,
			append([]interface{}{endpoint}, args...)...)
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		if addrErr, ok := err.(*net.AddrError); ok {
			return EP{}, mkErr("%s", addrErr.Err)
		}
		// shouldn't happen, but...
		return EP{}, mkErr("%s", err)
	}
	if host == "" {
		return EP{}, mkErr("invalid empty host")
	}
	if !hostRegex.MatchString(host) {
		return EP{}, mkErr("invalid host '%s'", host)
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return EP{}, mkErr("invalid port number '%s'", port)
	}
	return EP{host: host, port: uint16(portNum)}, nil
}

// Parse() is like ParseStricter(), but it disregards spaces before and after
// the endpoint string.
func Parse(endpoint string) (EP, error) {
	return ParseStricter(strings.TrimSpace(endpoint))
}

// MustParse() is similar to Parse(), but it panics if endpoint is invalid and
// can't be parsed. useful primarily for tests and global "consts" inits.
func MustParse(endpoint string) EP {
	ep, err := Parse(endpoint)
	if err != nil {
		panic(fmt.Sprintf("endpoint: Parse(%s): %s", endpoint, err.Error()))
	}
	return ep
}

func (ep EP) Host() string {
	return ep.host
}

func (ep EP) Port() uint16 {
	return ep.port
}

func (ep EP) PortString() string {
	return strconv.FormatUint(uint64(ep.port), 10)
}

func (ep EP) IsValid() bool {
	return ep != EP{}
}

func (ep EP) String() string {
	if !ep.IsValid() {
		return "<EMPTY>"
	}
	return net.JoinHostPort(ep.host, ep.PortString())
}

type Slice []EP

func (eps Slice) String() string {
	tgts := make([]string, len(eps))
	for i, ep := range eps {
		tgts[i] = ep.String()
	}
	return strings.Join(tgts, ",")
}

func (eps Slice) Equal(rhs Slice) bool {
	if len(eps) != len(rhs) {
		return false
	}
	for i, ep := range eps {
		if ep != rhs[i] {
			return false
		}
	}
	return true
}

func (eps Slice) IsValid() bool {
	for _, ep := range eps {
		if !ep.IsValid() {
			return false
		}
	}
	return len(eps) >= 1
}

func (eps Slice) Clone() Slice {
	res := make([]EP, len(eps))
	copy(res, eps)
	return res
}

// for sort.Interface:
func (l Slice) Len() int           { return len(l) }
func (l Slice) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l Slice) Less(i, j int) bool { return strings.Compare(l[i].String(), l[j].String()) == -1 }

func canonicalize(targets []string, parser func(string) (EP, error)) (Slice, error) {
	if len(targets) == 0 {
		return nil, nil
	}

	uniq := make(map[EP]bool)
	for _, tgt := range targets {
		ep, err := parser(tgt)
		if err != nil {
			return nil, err
		}
		uniq[ep] = true
	}

	res := make([]EP, 0, len(uniq))
	for k := range uniq {
		res = append(res, k)
	}
	sort.Sort(Slice(res))
	return res, nil
}

func ParseSlice(targets []string) (Slice, error) {
	return canonicalize(targets, Parse)
}

// ParseCSV() parses a string containing comma-separated list of target
// endpoints, validates them syntactically, and returns a slice of EP structs.
// it does NOT attempt to resolve the names present in the endpoints nor does
// it try to connect to any of the targets. `targets` must be in a format:
//     <host>:<port>[,<host>:<port>...]
func ParseCSV(endpoints string) (Slice, error) {
	return canonicalize(strings.Split(endpoints, ","), ParseStricter)
}

func MustParseCSV(endpoints string) Slice {
	eps, err := ParseCSV(endpoints)
	if err != nil {
		panic(fmt.Sprintf("endpoint: ParseCSV(%s): %s", endpoints, err.Error()))
	}
	return eps
}
