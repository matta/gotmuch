package persist

import (
	"math"
	"testing"
)

func TestOrdered(t *testing.T) {
	cases := []struct {
		u uint64
		s int64
	}{
		{0, math.MinInt64},
		{math.MaxUint64, math.MaxInt64},
		{math.MaxInt64 + 1, 0},
	}
	for _, tc := range cases {
		s := orderedToSigned(tc.u)
		if s != tc.s {
			t.Errorf("orderedToSigned(%x) = %x, want %x", tc.u, s, tc.s)
		}
		u := orderedToUnsigned(tc.s)
		if u != tc.u {
			t.Errorf("orderedToUnsigned(%x) = %x, want %x", tc.s, u, tc.u)
		}
	}
}
