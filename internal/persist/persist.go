// yo
package persist

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/matta/gotmuch/internal/message"
	"github.com/pkg/errors"
)

type DB struct {
	db *sql.DB
}

type Tx struct {
	tx *sql.Tx
}

// OpenDB TODO: write me
func Open(ctx context.Context, path string) (*DB, error) {
	// The _busy_timeout is a SQLite extension that controls how long SQLite will poll
	// before giving up.  The default of 5 seconds is too short in practice, especially
	// in slower debug builds; go with 5 minutes.
	var busyTimeout = int(5*time.Minute) / int(time.Millisecond)
	dsn := fmt.Sprintf("file:%s?_busy_timeout=%d", path, busyTimeout)
	fmt.Println("opening database at", dsn)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open database at %s", dsn)
	}

	if err = initSchema(ctx, db); err != nil {
		db.Close()
		return nil, errors.Wrapf(err, "could not init database schema")
	}

	return &DB{db}, nil
}

func (db *DB) Close() {
	db.db.Close()
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
	//   successfully for this message yet.
	//
	//   This field is set NULL for all messages before each GMail
	//   API "list" call, and populated on the subsequent "get".
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
	sql := `
CREATE TABLE IF NOT EXISTS gmail_messages (
message_id TEXT NOT NULL PRIMARY KEY,
thread_id TEXT NOT NULL,
history_id INTEGER,
size_estimate INTEGER
);`
	if _, err := db.ExecContext(ctx, sql); err != nil {
		return errors.Wrap(err, "could not create gmail_messages table")
	}

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
	sql = `
CREATE TABLE IF NOT EXISTS gmail_labels (
label_id TEXT NOT NULL PRIMARY KEY,
display_name TEXT NOT NULL,
type TEXT NOT NULL
);`
	if _, err := db.ExecContext(ctx, sql); err != nil {
		return errors.Wrap(err, "could not create gmail_labels table")
	}

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
	sql = `
CREATE TABLE IF NOT EXISTS gmail_message_labels (
message_id TEXT NOT NULL,
label_id TEXT NOT NULL,
PRIMARY KEY (message_id, label_id)
FOREIGN KEY (message_id) REFERENCES gmail_messages (message_id)
);`
	if _, err := db.ExecContext(ctx, sql); err != nil {
		return errors.Wrap(err, "could not create gmail_message_labels table")
	}

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
	sql = `
CREATE TABLE IF NOT EXISTS gmail_history_id (
history_id INTEGER NOT NULL,
PRIMARY KEY (history_id)
);`
	if _, err := db.ExecContext(ctx, sql); err != nil {
		return errors.Wrap(err, "could not create gmail_history_id table")
	}

	return nil
}

// InsertMessageID TODO: write me
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
