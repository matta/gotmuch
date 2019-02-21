package sync

import (
	"context"
	"fmt"

	"marmstrong/gotmuch/internal/message"
	"marmstrong/gotmuch/internal/notmuch"
	"marmstrong/gotmuch/internal/persist"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func listIds(ctx context.Context, g MessageStorage, msgs chan<- *message.ID) error {
	defer close(msgs)
	err := g.ListAll(ctx, func(msg *message.ID) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msgs <- msg:
			return nil
		}
	})
	if err != nil {
		err = errors.Wrap(err, "unable to retrieve all messages")
	}
	return err
}

func getMessageMeta(ctx context.Context, s MessageStorage, id string) (msg *message.Header, err error) {
	msg, err = s.GetMessageMeta(ctx, id)
	return
}

// func handleListedMessage(ctx context.Context, db *sql.DB, g MessageStorage, nm *notmuch.Service, msg *message.ID) error {
// 	if err := persist.InsertMessageID(ctx, db, msg); err != nil {
// 		return errors.Wrapf(err, "handling listed message")
// 	}

// 	// if nm.HaveMessage(msg.PermID) {
// 	// 	return nil
// 	// }

// 	// fullMsg, err := g.GetMessageFull(ctx, msg.PermID)
// 	// if err != nil {
// 	// 	return errors.Wrapf(err, "failed getting message %v", msg.PermID)
// 	// }
// 	// fmt.Println("Inserting ID", msg.PermID, "HistoryID", fullMsg.HistoryID, "SizeEstimate", fullMsg.SizeEstimate)
// 	// return nm.Insert(ctx, fullMsg)
// }

// func catchUp(ctx context.Context, s MessageStorage, db *sql.DB, nm *notmuch.Service) error {
// g, ctx := errgroup.WithContext(ctx)
// msgs := make(chan *message.ID)
// g.Go(func() error {
// 	return listMessages(ctx, s, msgs)
// })

// const concurrency = 100
// for i := 0; i < concurrency; i++ {
// 	msg, ok := <-msgs
// 	if !ok {
// 		break
// 	}
// 	g.Go(func() error {
// 		var err error
// 		for {
// 			err = handleListedMessage(ctx, db, s, nm, msg)
// 			if err != nil {
// 				break
// 			}
// 			msg, ok = <-msgs
// 			if !ok {
// 				break
// 			}
// 		}
// 		return errors.Wrap(err, "unable to handle listed message")
// 	})
// }

// return g.Wait()
// }

func saveIds(ctx context.Context, g MessageStorage, tx *persist.Tx,
	nm *notmuch.Service, ids <-chan *message.ID) error {
	// first := true
	for id := range ids {
		if err := tx.InsertMessageID(ctx, id); err != nil {
			return err
		}

		// TODO: re-enable this once syncIncrementalGmail() is written.
		// if !first {
		// 	continue
		// }
		// first = false
		// hdr, err := getMessageMeta(ctx, g, id.PermID)
		// if err != nil {
		// 	return err
		// }
		// if err = tx.WriteHistoryID(ctx, hdr.HistoryID); err != nil {
		// 	return err
		// }

		// TODO: move full message download elsewhere!
		if nm.HaveMessage(id.PermID) {
			continue
		}
		fullMsg, err := g.GetMessageFull(ctx, id.PermID)
		if err != nil {
			return errors.Wrapf(err, "failed getting message %v", id.PermID)
		}
		fmt.Println("Inserting ID", id.PermID, "HistoryID",
			fullMsg.HistoryID, "SizeEstimate", fullMsg.SizeEstimate)
		if err := nm.Insert(ctx, fullMsg); err != nil {
			return err
		}
	}
	return nil
}

func syncCatchUpGmail(ctx context.Context, g MessageStorage, tx *persist.Tx, nm *notmuch.Service) error {
	grp, ctx := errgroup.WithContext(ctx)
	ids := make(chan *message.ID, 1000)
	grp.Go(func() error {
		return listIds(ctx, g, ids)
	})
	grp.Go(func() error {
		return saveIds(ctx, g, tx, nm, ids)
	})
	return grp.Wait()
}

func syncIncrementalGmail(ctx context.Context, tx *persist.Tx) error {
	return errors.New("syncIncrementalGmail: not implemented")
}

func syncGmail(ctx context.Context, g MessageStorage, db *persist.DB, nm *notmuch.Service) error {
	tx, err := db.Begin(ctx)
	defer tx.Rollback()

	historyId, err := tx.LatestHistoryID(ctx)
	if err != nil {
		return err
	}
	if historyId == 0 {
		err = syncCatchUpGmail(ctx, g, tx, nm)
	} else {
		err = syncIncrementalGmail(ctx, tx)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func Sync(ctx context.Context, g MessageStorage, db *persist.DB, nm *notmuch.Service) error {
	if err := syncGmail(ctx, g, db, nm); err != nil {
		return err
	}
	return errors.New("sync.Sync: pulling messages not implemented")
}

// func getMessages(ctx context.Context, db *sql.DB, messages *gmail.UsersMessagesService) error {
// 	sql := `select message_id, downloaded from messages where
// 		history_id is null
// 		OR downloaded = 0;`
// 	rows, err := db.QueryContext(ctx, sql)
// 	if err != nil {
// 		return errors.Wrap(err, "db query failed")
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		var id string
// 		var downloaded bool
// 		if err := rows.Scan(&id, &downloaded); err != nil {
// 			return errors.Wrap(err, "db scan failed")
// 		}

// 		log.Infof("need to label message_id %#v downloaded %#v", id, downloaded)
// 	}
// 	if err = rows.Err(); err != nil {
// 		return errors.Wrap(err, "db rows failed")
// 	}

// 	return nil
// }
