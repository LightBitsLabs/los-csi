// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lightbitslabs/los-csi/pkg/lb"
)

func TestMergeNodeIPAcl(t *testing.T) {
	manyAddrs := make([]string, maxIPAclEntries)
	for i := range manyAddrs {
		manyAddrs[i] = fmt.Sprintf("10.0.0.%d", i+1)
	}

	testCases := []struct {
		name   string
		cur    []string
		addrs  []string
		result []string
		errStr string
	}{
		{
			name:   "first enrollment displaces ALLOW_NONE",
			cur:    []string{lb.ACLAllowNone},
			addrs:  []string{"10.0.0.1"},
			result: []string{"10.0.0.1"},
		},
		{
			name:   "empty IP-ACL",
			cur:    []string{},
			addrs:  []string{"10.0.0.1"},
			result: []string{"10.0.0.1"},
		},
		{
			name:  "externally set ALLOW_ANY is left untouched",
			cur:   []string{lb.ACLAllowAny},
			addrs: []string{"10.0.0.1"},
		},
		{
			name:  "node already enrolled is a no-op",
			cur:   []string{"10.0.0.1", "10.0.0.2"},
			addrs: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:   "second node joins the first",
			cur:    []string{"10.0.0.1"},
			addrs:  []string{"10.0.0.2"},
			result: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:   "partial overlap enrolls only the missing address",
			cur:    []string{"10.0.0.1", "10.0.0.2"},
			addrs:  []string{"10.0.0.2", "10.0.0.3"},
			result: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		},
		{
			name:   "duplicates in current ACL are collapsed",
			cur:    []string{"10.0.0.1", "10.0.0.1"},
			addrs:  []string{"10.0.0.2"},
			result: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:   "entry cap exceeded",
			cur:    manyAddrs,
			addrs:  []string{"10.0.1.1"},
			errStr: "limit is 16",
		},
		{
			name:  "at the cap with enrolled node is a no-op",
			cur:   manyAddrs,
			addrs: manyAddrs[:2],
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := mergeNodeIPAcl(tc.cur, tc.addrs)
			if tc.errStr != "" {
				require.ErrorContains(t, err, tc.errStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.result, res)
		})
	}
}
