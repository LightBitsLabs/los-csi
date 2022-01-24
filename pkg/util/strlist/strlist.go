// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package strlist

import (
	"sort"
)

func AreEqual(l, r []string) bool {
	if len(l) != len(r) {
		return false
	}
	for i, ll := range l {
		if ll != r[i] {
			return false
		}
	}
	return true
}

func CopyUniqueSorted(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	uniq := make(map[string]bool)
	for _, ss := range s {
		uniq[ss] = true
	}

	res := make([]string, 0, len(uniq))
	for k := range uniq {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}
