// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
