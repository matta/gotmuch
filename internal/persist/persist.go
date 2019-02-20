// yo
package persist

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matta/gotmuch/internal/message"
	"github.com/pkg/errors"
)

// OpenDB TODO: write me
func OpenDB(ctx context.Context, path string) (*sql.DB, error) {
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
-- Traks the state of messages in GMail.
CREATE TABLE IF NOT EXISTS gmail_messages (
message_id TEXT NOT NULL PRIMARY KEY,
thread_id TEXT NOT NULL,
history_id TEXT,
size_estimate INTEGER,
);`
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create messages table")
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
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create gmail_labels table")
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
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create gmail_labels table")
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
history_id TEXT NOT NULL,
PRIMARY KEY (history_id)
);`
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create gmail_history_id table")
	}

	return db, nil
}

// InsertMessageID TODO: write me
func InsertMessageID(ctx context.Context, db *sql.DB, msg *message.ID) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "db begin transaction failed")
	}
	defer tx.Rollback()

	sql := `INSERT OR REPLACE INTO gmail_messages
		(message_id, thread_id) values ($1, $2)
		ON CONFLICT (message_id)
		DO UPDATE SET (thread_id, history_id) = ($2, NULL)`
	upsert, err := tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for messages upsert")
	}
	defer upsert.Close()

	sql = `DELETE FROM gmail_message_labels WHERE message_id = $1`
	unlabel, err := tx.PrepareContext(ctx, sql)
	if err != nil {
		return errors.Wrap(err, "db prepare statement failed for unlabel")
	}
	defer unlabel.Close()

	_, err = upsert.ExecContext(ctx, msg.PermID, msg.ThreadID)
	if err != nil {
		return errors.Wrap(err, "db upsert failed")
	}
	_, err = unlabel.ExecContext(ctx, msg.PermID)
	if err != nil {
		return errors.Wrap(err, "db unlabel failed")
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "transaction commit failed")
	}
	return err
}
