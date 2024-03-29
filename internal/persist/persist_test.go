// Copyright 2022 Matt Armtrong
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
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/matta/gotmuch/internal/message"

	"github.com/google/go-cmp/cmp"
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

func FuzzOrdered(f *testing.F) {
	cases := []uint64{
		0, 1, 2, 3, 4, 5, 6, 7, math.MaxInt64 - 1, math.MaxInt64, math.MaxInt64 + 1,
		math.MaxUint64 - 1, math.MaxUint64}
	for _, u := range cases {
		for _, inc := range cases {
			f.Add(u, inc)
		}
	}
	f.Fuzz(func(t *testing.T, u uint64, inc uint64) {
		s := orderedToSigned(u)
		if u != orderedToUnsigned(s) {
			t.Errorf("round trip failure %x -> %x -> %x",
				u, s, orderedToUnsigned(s))
		}
		uinc := u + inc
		sinc := orderedToSigned(uinc)
		if (u < uinc) != (s < sinc) {
			t.Errorf("order inversion: (%x < %x) != (%x < %x)",
				u, uinc, s, sinc)
		}
	})
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

type dbFixture struct {
	t      *testing.T
	tmpdir string
	db     *DB
}

type fixtureMode int

var (
	inMemorySequence int
)

const (
	inMemory fixtureMode = iota
	onDisk
)

func runEachMode(t *testing.T, test func(t *testing.T, mode fixtureMode)) {
	cases := []struct {
		name string
		mode fixtureMode
	}{
		{"in memory", inMemory},
		{"on disk", onDisk},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			test(t, c.mode)
		})
	}
}

func createDBFixture(ctx context.Context, mode fixtureMode, t *testing.T) *dbFixture {
	t.Helper()
	var tmpdir string
	var dsn string

	switch mode {
	case inMemory:
		inMemorySequence++
		dsn = fmt.Sprintf("file:memory_db_%d?mode=memory&cache=shared",
			inMemorySequence)
	case onDisk:
		tmpdir, err := ioutil.TempDir("", "test")
		if err != nil {
			t.Fatalf("ioutil.TempDir() error %v", err)
		}
		dsn = filepath.Join(tmpdir, "db")
	}

	t.Logf("Database is %q", dsn)
	db, err := Open(ctx, dsn)
	if err != nil {
		os.RemoveAll(tmpdir)
		t.Fatalf("persist.Open(%q) error %v", dsn, err)
	}
	return &dbFixture{t, tmpdir, db}
}

func (f *dbFixture) CloseOrFatal() {
	f.t.Helper()
	defer func() {
		if f.tmpdir == "" {
			return
		}
		if err := os.RemoveAll(f.tmpdir); err != nil {
			f.t.Fatalf("os.RemoveAll(%q) error %v", f.tmpdir, err)
		}
	}()
	if err := f.db.Close(); err != nil {
		f.t.Errorf("db.Close() error: %v", err)
	}
}

func (f *dbFixture) BeginOrFatal(ctx context.Context) *Tx {
	f.t.Helper()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		f.t.Fatalf("persist.DB.Begin() failes with error: %v", err)
	}
	return tx
}

func RollbackOrFatal(t *testing.T, tx *Tx) {
	if err := tx.Rollback(); err != nil {
		t.Fatalf("tx.Rollback() error %v", err)
	}
}

func CommitOrFatal(t *testing.T, tx *Tx) {
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit() error %v", err)
	}
}

func (f *dbFixture) ListUpdated(ctx context.Context, account string) map[string]message.ID {
	tx := f.BeginOrFatal(ctx)
	defer RollbackOrFatal(f.t, tx)

	m := map[string]message.ID{}
	err := tx.ListUpdated(ctx, account, 100, func(id message.ID) error {
		_, ok := m[id.PermID]
		if ok {
			f.t.Errorf("persist.Tx.ListUpdated() returned duplicate message.ID %#v", id)
			return nil
		}
		m[id.PermID] = id
		return nil
	})
	if err != nil {
		f.t.Fatalf("persist.Tx.ListUpdated() fails with error: %v", err)
	}
	return m
}

func TestDBFixture(t *testing.T) {
	runEachMode(t, func(t *testing.T, mode fixtureMode) {
		createDBFixture(context.Background(), mode, t).CloseOrFatal()
	})
}

func TestBeginRollback(t *testing.T) {
	runEachMode(t, func(t *testing.T, mode fixtureMode) {
		ctx := context.Background()
		fixture := createDBFixture(ctx, mode, t)
		tx := fixture.BeginOrFatal(ctx)
		defer fixture.CloseOrFatal()
		RollbackOrFatal(t, tx)
	})
}

func testBeginCommit(t *testing.T, mode fixtureMode) {
	ctx := context.Background()
	fixture := createDBFixture(ctx, mode, t)
	defer fixture.CloseOrFatal()
	tx := fixture.BeginOrFatal(ctx)
	CommitOrFatal(t, tx)
}
func TestBeginCommit(t *testing.T) {
	runEachMode(t, testBeginCommit)
}

func testInsertMessageID(t *testing.T, mode fixtureMode) {
	ctx := context.Background()
	fixture := createDBFixture(ctx, mode, t)
	defer fixture.CloseOrFatal()

	const account = "account"
	tx := fixture.BeginOrFatal(ctx)
	defer tx.Rollback()
	for _, msg := range []message.ID{
		message.ID{"m1", "t1"},
		message.ID{"m2", "t2"},
		message.ID{"m1", "t1"},
	} {
		if err := tx.InsertMessageID(ctx, account, msg); err != nil {
			t.Fatalf("tx.InsertMessageID() error: %+v", err)
		}
	}
	CommitOrFatal(t, tx)

	got := fixture.ListUpdated(ctx, account)
	want := map[string]message.ID{"m1": {"m1", "t1"}, "m2": {"m2", "t2"}}
	if !cmp.Equal(got, want) {
		t.Errorf("persist.Tx.ListUpdated() = %v, want %v, diff %s",
			got, want, cmp.Diff(got, want))
	}
}

func TestInsertMessageID(t *testing.T) {
	runEachMode(t, testInsertMessageID)
}

func testUpdateHeader(t *testing.T, mode fixtureMode) {
	ctx := context.Background()
	fixture := createDBFixture(ctx, mode, t)
	defer fixture.CloseOrFatal()

	tx := fixture.BeginOrFatal(ctx)
	defer tx.Rollback()
	id := message.ID{"m1", "t1"}
	const account = "account"
	tx.InsertMessageID(ctx, account, id)
	CommitOrFatal(t, tx)

	tx = fixture.BeginOrFatal(ctx)
	defer tx.Rollback()
	hdr := message.Header{
		ID:           id,
		LabelIDs:     []string{"label_a", "label_b"},
		SizeEstimate: 1234,
		HistoryID:    13579,
	}
	err := tx.UpdateHeader(ctx, account, &hdr)
	if err != nil {
		t.Fatalf("tx.UpdateHeader(%v) error: %+v", hdr, err)
	}
	CommitOrFatal(t, tx)

	// TODO: add tests when persist gets an API to read these
	// messages back.
}

func TestUpdateHeader(t *testing.T) {
	runEachMode(t, testUpdateHeader)
}

func testHistoryID(t *testing.T, mode fixtureMode) {
	ctx := context.Background()
	fixture := createDBFixture(ctx, mode, t)
	defer fixture.CloseOrFatal()

	tx := fixture.BeginOrFatal(ctx)
	id, err := tx.LatestHistoryID(ctx)
	if err != nil {
		t.Fatalf("persist.Tx.LatestHistoryID() "+
			"unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("persist.Tx.LatestHistoryID() = %v"+
			", want 0 (because no prior historyID"+
			"has been commited)", id)
	}

	const fakeID = 12345
	err = tx.WriteHistoryID(ctx, "account", fakeID)
	if err != nil {
		t.Fatalf("WriteHistoryID() unexpected error: %v", err)
	}
	CommitOrFatal(t, tx)

	tx = fixture.BeginOrFatal(ctx)
	defer RollbackOrFatal(t, tx)
	id, err = tx.LatestHistoryID(ctx)
	if err != nil {
		t.Fatalf("LatestHistoryID() unexpected error: %v", err)
	}
	if id != fakeID {
		t.Errorf("LatestHistoryID() = %d, want %d", id, fakeID)
	}
}

func TestHistoryID(t *testing.T) {
	runEachMode(t, testHistoryID)
}
