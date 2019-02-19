package sync

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matta/gotmuch/internal/message"
	"github.com/matta/gotmuch/internal/notmuch"
	"github.com/matta/gotmuch/internal/persist"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func handleListedMessage(ctx context.Context, db *sql.DB, g MessageStorage, nm *notmuch.Service, msg *message.ID) error {
	if err := persist.InsertMessageID(ctx, db, msg); err != nil {
		return errors.Wrapf(err, "handling listed message")
	}

	if nm.HaveMessage(msg.PermID) {
		return nil
	}

	fullMsg, err := g.GetMessageFull(ctx, msg.PermID)
	if err != nil {
		return errors.Wrapf(err, "failed getting message %v", msg.PermID)
	}
	fmt.Println("Inserting ID", msg.PermID, "HistoryID", fullMsg.HistoryID, "SizeEstimate", fullMsg.SizeEstimate)
	return nm.Insert(ctx, fullMsg)
}

func listMessages(ctx context.Context, s MessageStorage, msgs chan<- *message.ID) error {
	defer close(msgs)
	err := s.ListAll(ctx, func(msg *message.ID) error {
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

func CatchUp(ctx context.Context, s MessageStorage, db *sql.DB, nm *notmuch.Service) error {
	g, ctx := errgroup.WithContext(ctx)
	msgs := make(chan *message.ID)
	g.Go(func() error {
		return listMessages(ctx, s, msgs)
	})

	const concurrency = 100
	for i := 0; i < concurrency; i++ {
		msg, ok := <-msgs
		if !ok {
			break
		}
		g.Go(func() error {
			var err error
			for {
				err = handleListedMessage(ctx, db, s, nm, msg)
				if err != nil {
					break
				}
				msg, ok = <-msgs
				if !ok {
					break
				}
			}
			return errors.Wrap(err, "unable to handle listed message")
		})
	}

	return g.Wait()
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
