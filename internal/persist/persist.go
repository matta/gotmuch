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
	"database/sql"
	"fmt"
	"log"
	"math"
	"net/url"
	"strings"
	"time"

	"marmstrong/gotmuch/internal/message"

	"github.com/pkg/errors"
)

var (
	createTableSql = []string{
		// The gmail_messages table holds state for each message in
		// the database.
		//
		// Field: message_id
		//
		//   GMail API: Users.messages resource "id" field, returned
		//   by Users.messages.list and Users.messages.get (for all
		//   formats).
		//
		//   Note: This is also exposed by GMail IMAP as the
		//   X-GM-MSGID header, where it is documented as a uint64
		//   integer encoded in hex.
		//
		// Field: thread_id
		//
		//   GMail API: Users.messages resource "threadId" field,
		//   returned by Users.messages.list, Users.messages.get (for
		//   all formats).
		//
		//   Note: This is also exposed by GMail IMAP as the
		//   X-GM-THRID header, where it is documented as a uint64
		//   integer encoded in hex.
		//
		// Field: history_id
		//
		//   GMail API: Users.messages resource "historyId" field,
		//   returned by Users.messages.get for all formats.
		//
		//   Notes:
		//
		//   If NULL, no Users.messages.get has been performed
		//   successfully for this message yet, or the message_id has
		//   appeared in a Users.history.list response.
		//
		//   This field is set NULL for all messages before each GMail
		//   API "list" call, and populated on the subsequent "get".
		//   It is set NULL when a Users.history.list response
		//   includes the message_id.
		//
		// Field: size_estimate
		//
		//   GMail API: Users.messages resource "sizeEstimate" field,
		//   returned by Users.messages.get for all formats.
		//
		//   Notes:
		//
		//   If NULL, no Users.messages.get has been performed
		//   successfully for this message yet.
		//
		//   This field is never set NULL.  Once fetched it is
		//   considered valid for the message_id for the life of the
		//   database.
		`
CREATE TABLE IF NOT EXISTS gmail_messages (
message_id TEXT NOT NULL PRIMARY KEY,
thread_id TEXT NOT NULL,
history_id INTEGER,
size_estimate INTEGER
);`,
		// The gmail_labels table maps label IDs to display name and
		// type.
		//
		// Field: label_id
		//
		//   GMail API: Users.labels resource "id"
		//
		// Field: display_name
		//
		//   GMail API: Users.labels resource "name"
		//
		// Field: type
		//
		//   GMail API: Users.labels resource "type"
		//   Valid values are "system" or "user".
		`
CREATE TABLE IF NOT EXISTS gmail_labels (
label_id TEXT NOT NULL PRIMARY KEY,
display_name TEXT NOT NULL,
type TEXT NOT NULL
);`,
		// The gmail_message_labels table maps messages to labels.
		//
		//   Maps gmail message ID to a label_id.
		//
		// Field: message_id
		//
		//   As in gmail_messages.message_id.
		//
		// Field: label_id
		//
		//   As in gmail_labels.label_id.
		`
CREATE TABLE IF NOT EXISTS gmail_message_labels (
message_id TEXT NOT NULL,
label_id TEXT NOT NULL,
PRIMARY KEY (message_id, label_id)
FOREIGN KEY (message_id) REFERENCES gmail_messages (message_id)
);`,
		// The gmail_history_id table holds the GMail history ID for
		// each successful full synchronizaton, either a complete call
		// to Users.messages.list (catch up synchronizaton) or
		// Users.history.list (incremental synchronizaton).
		//
		// Notes:
		//
		// The highest ID (sorted lexicographically) is the latest
		// history ID for which history has been synchronized.
		//
		// All rows in this table are erased before each
		// Users.messages.list call (catch up synchronization).
		`
CREATE TABLE IF NOT EXISTS gmail_history_id (
history_id INTEGER NOT NULL,
PRIMARY KEY (history_id)
);`,
	}
)

type DB struct {
	db *sql.DB
}

type Tx struct {
	tx *sql.Tx
}

func dsnFromPath(path string, addValues url.Values) (string, error) {
	var u *url.URL
	if !strings.HasPrefix(path, "file:") {
		u = &url.URL{Scheme: "file", Path: path}
	} else {
		var err error
		u, err = url.Parse(path)
		if err != nil {
			return "", err
		}
	}
	values := u.Query()
	for k, v := range addValues {
		for _, item := range v {
			values.Add(k, item)
		}
	}
	u.RawQuery = values.Encode()
	return u.String(), nil
}

func Open(ctx context.Context, path string) (*DB, error) {
	// The _busy_timeout is a SQLite extension that controls how
	// long SQLite will poll before giving up.  The default of 5
	// seconds is too short in practice, especially in slower
	// debug builds; go with 5 minutes.
	var busyTimeout = int(5*time.Minute) / int(time.Millisecond)

	dsn, err := dsnFromPath(path, url.Values{
		"_busy_timeout": {fmt.Sprintf("%d", busyTimeout)}})
	if err != nil {
		return nil, errors.Wrapf(err,
			"Open(%q) failed: could not form a DB DSN from "+
				"the given path",
			path)
	}
	log.Printf("opening database at %q\n", dsn)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, errors.Wrapf(err,
			"Open(%q) failed: could not open database at %q",
			path, dsn)
	}

	if err = initSchema(ctx, db); err != nil {
		db.Close()
		return nil, errors.Wrapf(err,
			"Open(%q) failed: could not initialize the "+
				"database schema", path)
	}

	return &DB{db}, nil
}

func (db *DB) Close() error {
	return db.db.Close()
}

func (db *DB) Begin(ctx context.Context) (*Tx, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "begin transaction failed")
	}
	return &Tx{tx}, nil
}

func (tx *Tx) Commit() error {
	return tx.tx.Commit()
}

func (tx *Tx) Rollback() error {
	return tx.tx.Rollback()
}

func initSchema(ctx context.Context, db *sql.DB) error {
	for _, sql := range createTableSql {
		log.Printf("SQL Exec: %q", sql)
		if _, err := db.ExecContext(ctx, sql); err != nil {
			return errors.Wrapf(err, "while executing %q", sql)
		}
	}

	return nil
}

func (tx *Tx) InsertMessageID(ctx context.Context, msg *message.ID) error {
	sql := `INSERT OR REPLACE INTO gmail_messages
		(message_id, thread_id) values ($1, $2)
		ON CONFLICT (message_id)
		DO UPDATE SET (thread_id, history_id) = ($2, NULL)`
	upsert, err := tx.tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for messages upsert")
	}
	defer upsert.Close()

	sql = `DELETE FROM gmail_message_labels WHERE message_id = $1`
	unlabel, err := tx.tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for unlabel")
	}
	defer unlabel.Close()

	if _, err = upsert.ExecContext(ctx, msg.PermID, msg.ThreadID); err != nil {
		return errors.Wrap(err, "db upsert failed")
	}
	if _, err = unlabel.ExecContext(ctx, msg.PermID); err != nil {
		return errors.Wrap(err, "db unlabel failed")
	}
	return nil
}

func (tx *Tx) UpdateHeader(ctx context.Context, hdr *message.Header) error {
	sql := `
UPDATE gmail_messages SET (history_id, size_estimate) = ($1, $2)
WHERE message_id = $3;
`
	history, err := tx.tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for UpdateHeader")
	}
	defer history.Close()

	sql = `
DELETE FROM gmail_message_labels WHERE message_id = $1
`
	unlabel, err := tx.tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for unlabel")
	}
	defer unlabel.Close()

	sql = `
INSERT INTO gmail_message_labels (message_id, label_id) values ($1, $2)
`
	label, err := tx.tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for unlabel")
	}
	defer label.Close()

	if _, err = history.ExecContext(ctx, hdr.HistoryID, hdr.SizeEstimate, hdr.ID.PermID); err != nil {
		return errors.Wrap(err, "UpdateHeader")
	}
	if _, err = unlabel.ExecContext(ctx, hdr.ID.PermID); err != nil {
		return errors.Wrap(err, "UpdateHeader")
	}
	for _, labelID := range hdr.LabelIDs {
		if _, err = label.ExecContext(ctx, hdr.ID.PermID, labelID); err != nil {
			return errors.Wrap(err, "UpdateHeader")
		}
	}
	return nil
}

func (tx *Tx) ListUpdated(ctx context.Context, handler func(message.ID) error) error {
	const sql = `
SELECT message_id, thread_id
FROM gmail_messages
WHERE history_id IS NULL
`
	rows, err := tx.tx.QueryContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed in ListOutdatedHeaders")
	}
	defer rows.Close()

	for rows.Next() {
		var permID string
		var threadID string
		if err := rows.Scan(&permID, &threadID); err != nil {
			return errors.Wrap(err, "db scan failed in ListOutdatedHeaders")
		}
		if err := handler(message.ID{PermID: permID, ThreadID: threadID}); err != nil {
			return err
		}
	}
	return nil
}

func formatId(id uint64) string {
	return fmt.Sprintf("%016x", id)
}

func orderedToSigned(u uint64) int64 {
	return int64(u - -math.MinInt64) // Imagine 0..255 -> -128..127
}

func orderedToUnsigned(s int64) uint64 {
	return uint64(s) + -math.MinInt64 // Imagine -128..127 -> 0..255
}

func (tx *Tx) LatestHistoryID(ctx context.Context) (uint64, error) {
	const q = `SELECT history_id FROM gmail_history_id ORDER BY history_id DESC LIMIT 1`
	row := tx.tx.QueryRowContext(ctx, q)
	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			err = nil // a non-error
		}
		return 0, err
	}
	return orderedToUnsigned(id), nil
}

func (tx *Tx) WriteHistoryID(ctx context.Context, history_id uint64) error {
	latest, err := tx.LatestHistoryID(ctx)
	if err != nil {
		return err
	}
	if history_id <= latest {
		return fmt.Errorf("attempt to decrease the latest history_id")
	}

	sql := `INSERT INTO gmail_history_id (history_id) values ($1)`
	_, err = tx.tx.ExecContext(ctx, sql, orderedToSigned(history_id))
	if err != nil {
		return errors.Wrap(err, "db insert failed")
	}
	return nil
}
