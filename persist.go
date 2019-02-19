// yo
package persist

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"google3/experimental/users/marmstrong/gotmuch/message"
	"google3/third_party/golang/errors/errors"
)

// OpenDB TODO: write me
func OpenDB(ctx context.Context, path string) (db *sql.DB, err error) {
	// The _busy_timeout is a SQLite extension that controls how long SQLite will poll
	// before giving up.  The default of 5 seconds is too short in practice, especially
	// in slower debug builds; go with 5 minutes.
	var busyTimeout = int(5*time.Minute) / int(time.Millisecond)
	dsn := fmt.Sprintf("file:%s?_busy_timeout=%d", path, busyTimeout)
	fmt.Println("opening database at", dsn)
	db, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open database at %s", dsn)
	}

	sql := `
-- Traks the state of messages in GMail.
CREATE TABLE IF NOT EXISTS gmail_messages (
	-- GMail API: Users.messages resource "id" field
	-- Returned by "list", "get" (always)
	-- Note: this is also exposed by GMail IMAP as the X-GM-MSGID
	--       header, where it is documented as a uint64 integer
	--       encoded in hex.
	message_id TEXT NOT NULL PRIMARY KEY,

	-- GMail API: Users.messages resource "threadId" field
	-- Returned by "list", "get" (always)
	-- Note: this is also exposed by GMail IMAP as the X-GM-THRID
	--       header, where it is documented as a uint64 integer
	--       encoded in hex.
	thread_id TEXT NOT NULL,

	-- GMail API: Users.messages resource "historyId" field
	-- Returned by "get" for all formats
	history_id TEXT,

	-- GMail API: Users.messages resource "sizeEstimate" field
	-- Returned by "get" for all formats
	size_estimate INTEGER
);`
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create messages table")
	}

	sql = `
CREATE TABLE IF NOT EXISTS gmail_message_labels (
	-- GMail API: Users.messages resource "id" field
	message_id TEXT NOT NULL,

	-- GMail API: Users.labels resource "id"
	label_id TEXT NOT NULL,

	PRIMARY KEY (message_id, label_id)
	FOREIGN KEY (message_id) REFERENCES gmail_messages (message_id)
);`
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create gmail_labels table")
	}

	sql = `
CREATE TABLE IF NOT EXISTS gmail_labels (
	-- GMail API: Users.labels resource "id"
	label_id TEXT NOT NULL PRIMARY KEY,

	-- GMail API: Users.labels resource "nae"
	display_name TEXT NOT NULL,

	-- GMail API: Users.labels resource "type" field.
	-- Valid values: "system" and "user"
	type TEXT NOT NULL
);`
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		db.Close()
		return nil, errors.Wrap(err, "could not create gmail_labels table")
	}

	return
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
