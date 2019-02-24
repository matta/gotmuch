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

package sync

import (
	"context"
	"fmt"
	"log"

	"marmstrong/gotmuch/internal/message"
	"marmstrong/gotmuch/internal/notmuch"
	"marmstrong/gotmuch/internal/persist"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/googleapi"
)

func listIds(ctx context.Context, historyId uint64, g MessageStorage, msgs chan<- *message.ID) error {
	defer close(msgs)

	if historyId == 0 {
		err := g.ListAll(ctx, func(msg *message.ID) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case msgs <- msg:
				return nil
			}
		})
		if err != nil {
			return errors.Wrap(err, "unable to retrieve all messages")
		}
		return nil
	}
	err := g.ListFrom(ctx, historyId, func(msg *message.ID) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msgs <- msg:
			return nil
		}
	})
	if err != nil {
		return errors.Wrap(err, "unable to retrieve incremental messages")
	}
	return nil

}

func getMessageMeta(ctx context.Context, s MessageStorage, id string) (msg *message.Header, err error) {
	msg, err = s.GetMessageMeta(ctx, id)
	return
}

func saveIds(ctx context.Context, tx *persist.Tx, ids <-chan *message.ID) error {
	for id := range ids {
		if err := tx.InsertMessageID(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func pullComplete(ctx context.Context, g MessageStorage, tx *persist.Tx) error {
	profile, err := g.GetProfile(ctx)
	if err != nil {
		return err
	}
	log.Println("Full sync to History ID", profile.HistoryID, "for", profile.EmailAddress)
	err = tx.WriteHistoryID(ctx, profile.HistoryID)
	if err != nil {
		return err
	}

	grp, ctx := errgroup.WithContext(ctx)
	ids := make(chan *message.ID, 1000)
	grp.Go(func() error {
		return listIds(ctx, 0, g, ids)
	})
	grp.Go(func() error {
		return saveIds(ctx, tx, ids)
	})
	return grp.Wait()
}

func pullIncremental(ctx context.Context, historyID uint64, g MessageStorage, tx *persist.Tx) error {
	profile, err := g.GetProfile(ctx)
	if err != nil {
		return err
	}
	log.Println("Incremental sync from", historyID, "for", profile.EmailAddress)
	if historyID == profile.HistoryID {
		return nil
	}
	if historyID > profile.HistoryID {
		// TODO: handle history ID reset
		return errors.New("Not implemented: history ID has been reset!")
	}

	// TODO: can we trust this history ID here?
	err = tx.WriteHistoryID(ctx, profile.HistoryID)
	if err != nil {
		return err
	}

	grp, ctx := errgroup.WithContext(ctx)
	ids := make(chan *message.ID, 1000)
	grp.Go(func() error {
		return listIds(ctx, historyID, g, ids)
	})
	grp.Go(func() error {
		return saveIds(ctx, tx, ids)
	})
	return grp.Wait()
}

func pullList(ctx context.Context, g MessageStorage, db *persist.DB, nm *notmuch.Service) error {
	tx, err := db.Begin(ctx)
	defer tx.Rollback()

	historyId, err := tx.LatestHistoryID(ctx)
	if err != nil {
		return err
	}
	if historyId == 0 {
		err = pullComplete(ctx, g, tx)
	} else {
		err = pullIncremental(ctx, historyId, g, tx)
	}
	if err != nil {
		return errors.Wrap(err, "failed to list messages in pullList()")
	}

	return tx.Commit()
}

func pullDownload(ctx context.Context, g MessageStorage, db *persist.DB, nm *notmuch.Service) error {
	tx, err := db.Begin(ctx)
	defer tx.Rollback()

	grp, ctx := errgroup.WithContext(ctx)
	ids := make(chan message.ID)

	grp.Go(func() error {
		defer close(ids)
		return tx.ListOutdated(ctx, func(id message.ID) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ids <- id:
				return nil
			}
		})
	})

	const concurrency = 1
	for i := 0; i < concurrency; i++ {
		id, ok := <-ids
		if !ok {
			break
		}
		grp.Go(func() error {
			for {
				if err = handleOutdatedMessage(ctx, tx, g, nm, id); err != nil {
					errors.Wrap(err, "unable to handle outdated message")
				}
				id, ok = <-ids
				if !ok {
					return nil
				}
			}
		})
	}

	if err := grp.Wait(); err != nil {
		return errors.Wrap(err, "unable to pull outdated messages")
	}
	return tx.Commit()
}

func handleOutdatedMessage(ctx context.Context, tx *persist.Tx, g MessageStorage, nm *notmuch.Service, id message.ID) error {
	// TODO: move full message download elsewhere once we're tracking labels.
	if nm.HaveMessage(id.PermID) {
		return nil
	}
	fullMsg, err := g.GetMessageFull(ctx, id.PermID)

	// TODO: save history ID and label information here.

	if err != nil {
		// In practice the history list sometimes delivers
		// messages that can't be fetched; ignore them.
		//
		// TODOD: Handle this more gracefully by generalizing
		// a "not found" error in the MessageStorage
		// interface?
		switch err := errors.Cause(err).(type) {
		case *googleapi.Error:
			if err.Code == 404 {
				return nil
			}
		}
		return errors.Wrapf(err, "failed getting message %v", id.PermID)
	}
	fmt.Println("Inserting ID", id.PermID, "HistoryID",
		fullMsg.HistoryID, "SizeEstimate", fullMsg.SizeEstimate)
	if err := nm.Insert(ctx, fullMsg); err != nil {
		return err
	}
	return nil
}

func Sync(ctx context.Context, g MessageStorage, db *persist.DB, nm *notmuch.Service) error {
	log.Print("Pulling list of GMail messages")
	if err := pullList(ctx, g, db, nm); err != nil {
		return errors.Wrap(err, "failed to sync")
	}
	log.Print("Pulling GMail messages")
	if err := pullDownload(ctx, g, db, nm); err != nil {
		return errors.Wrap(err, "failed to sync")
	}
	return nil
}
