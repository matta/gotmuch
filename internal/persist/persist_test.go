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
	"context"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
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

func TestDSN(t *testing.T) {
	cases := []struct {
		path   string
		values url.Values
		dsn    string
	}{
		{
			"",
			url.Values{},
			"file:",
		},
		{
			"foo",
			url.Values{},
			"file://foo",
		},
		{
			"./foo",
			url.Values{},
			"file://./foo",
		},
		{
			"/foo",
			url.Values{},
			"file:///foo",
		},
		{
			":memory:",
			url.Values{},
			"file://:memory:",
		},
		{
			"p",
			url.Values{"q": {"42"}},
			"file://p?q=42",
		},
		{
			"file:p?q=43",
			url.Values{"q": {"42"}},
			"file:p?q=43&q=42",
		},
		{
			// This is an example of a programmer or
			// coding error.  Without the file: schema the
			// entire string is considered a path name.
			":memory:?mode=memory&cache=shared",
			url.Values{"q": {"42"}},
			"file://:memory:%3Fmode=memory&cache=shared?q=42",
		},
		{
			// This is an example of correct usage.
			"file::memory:?mode=memory&cache=shared",
			url.Values{"q": {"42"}},
			"file::memory:?cache=shared&mode=memory&q=42",
		},
		{
			// This is an example of correct usage.
			"file://:memory:?mode=memory&cache=shared",
			url.Values{"q": {"42"}},
			"file://:memory:?cache=shared&mode=memory&q=42",
		},
	}
	for _, tc := range cases {
		dsn, err := dsnFromPath(tc.path, tc.values)
		if err != nil {
			t.Errorf("dsnFromPath(%q, %#v) -> error: %v",
				tc.path, tc.values, err)
			continue
		}
		if dsn != tc.dsn {
			t.Errorf("dsnFromPath(%q, %#v) = %q, want %q",
				tc.path, tc.values, dsn, tc.dsn)
		}
	}
}

type fixture struct {
	t      *testing.T
	tmpdir string
	db     *DB
}

type fixtureMode int

const (
	inMemory fixtureMode = iota
	temporaryOnDisk
)

func createFixture(ctx context.Context, mode fixtureMode, t *testing.T) *fixture {
	t.Helper()
	var tmpdir string
	var dsn string

	switch mode {
	case inMemory:
		dsn = "file::memory:?mode=memory&cache=shared"
	case temporaryOnDisk:
		tmpdir, err := ioutil.TempDir("", "test")
		if err != nil {
			t.Fatalf("ioutil.TempDir() error %v", err)
		}
		dsn = filepath.Join(tmpdir, "db")
	}

	db, err := Open(ctx, dsn)
	if err != nil {
		os.RemoveAll(tmpdir)
		t.Fatalf("persist.Open(%q) error %v", dsn, err)
	}
	return &fixture{t, tmpdir, db}
}

func (f *fixture) Close() {
	f.db.Close()
	if f.tmpdir == "" {
		return
	}
	if err := os.RemoveAll(f.tmpdir); err != nil {
		f.t.Fatalf("os.RemoveAll(%q) error %v", f.tmpdir, err)
	}
}

func TestFixture(t *testing.T) {
	createFixture(context.Background(), inMemory, t).Close()
	createFixture(context.Background(), temporaryOnDisk, t).Close()
}
