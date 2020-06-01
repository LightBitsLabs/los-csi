package strlist_test

import (
	"fmt"
	"testing"

	"github.com/lightbitslabs/lb-csi/pkg/util/strlist"
)

type test struct {
	eq  bool
	in  []string
	out []string
}

var eqTests = []test{
	{eq: true, in: nil, out: nil},
	{eq: true, in: []string{}, out: nil},
	{eq: true, in: nil, out: []string{}},
	{eq: false, in: []string{}, out: make([]string, 1)},
	{eq: true, in: []string{""}, out: make([]string, 1)},
	{eq: false, in: []string{""}, out: make([]string, 0, 1)},
	{eq: false, in: []string{"a1"}, out: []string{}},
	{eq: false, in: []string{"a1"}, out: make([]string, 1)},
	{eq: false, in: []string{}, out: []string{"a1"}},
	{eq: true, in: []string{"a1"}, out: []string{"a1"}},
	{eq: true, in: []string{"a1", "a2"}, out: []string{"a1", "a2"}},
	{eq: false, in: []string{"a2", "a1"}, out: []string{"a1", "a2"}},
	{eq: false, in: []string{"a1", "a1"}, out: []string{"a1"}},
	{eq: false, in: []string{"a1"}, out: []string{"a1", "a1"}},
	{eq: false, in: []string{"a1", "a2", "a2"}, out: []string{"a1", "a2"}},
	{eq: false, in: []string{"a1", "a2", "a1"}, out: []string{"a2", "a1", "a1"}},
	{eq: false, in: []string{"a1", "a1", "a1"}, out: []string{"a1"}},
	{eq: true, in: []string{"a1", "a2", "a3"}, out: []string{"a1", "a2", "a3"}},
	{eq: false, in: []string{"a2", "a1", "a3"}, out: []string{"a1", "a2", "a3"}},
}

func TestAreEqual(t *testing.T) {
	for i, test := range eqTests {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			res := strlist.AreEqual(test.in, test.out)
			if res != test.eq {
				t.Errorf("equality comparison failed: got %t instead of %t:\n"+
					"in:  %q\nout: %q", res, test.eq, test.in, test.out)
			}
		})
	}
}

var cpTests = []test{
	{in: nil, out: nil},
	{in: []string{}, out: nil},
	{in: []string{"a1"}, out: []string{"a1"}},
	{in: []string{"a1", "a2"}, out: []string{"a1", "a2"}},
	{in: []string{"a2", "a1"}, out: []string{"a1", "a2"}},
	{in: []string{"a1", "a1"}, out: []string{"a1"}},
	{in: []string{"a1", "a2", "a2"}, out: []string{"a1", "a2"}},
	{in: []string{"a1", "a2", "a1"}, out: []string{"a1", "a2"}},
	{in: []string{"a2", "a2", "a1"}, out: []string{"a1", "a2"}},
	{in: []string{"a2", "a1", "a2"}, out: []string{"a1", "a2"}},
	{in: []string{"a2", "a1", "a1"}, out: []string{"a1", "a2"}},
	{in: []string{"a1", "a1", "a1"}, out: []string{"a1"}},
	{in: []string{"a1", "a2", "a3"}, out: []string{"a1", "a2", "a3"}},
	{in: []string{"a2", "a1", "a3"}, out: []string{"a1", "a2", "a3"}},
	{in: []string{"a3", "a2", "a1"}, out: []string{"a1", "a2", "a3"}},
	{in: []string{"a2", "", "a1"}, out: []string{"", "a1", "a2"}},
}

func TestCopyUniqueSorted(t *testing.T) {
	for i, test := range cpTests {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			res := strlist.CopyUniqueSorted(test.in)
			if !strlist.AreEqual(res, test.out) {
				t.Errorf("lists differ:\nin:  %q\nexp: %q\nres: %q",
					test.in, test.out, res)
			}
		})
	}
}
