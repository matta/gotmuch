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

package gmail

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"

	"marmstrong/gotmuch/internal/message"

	"github.com/pkg/errors"
	"golang.org/x/time/rate"
	"google.golang.org/api/gmail/v1"
	gmail_api "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

// "google3/util/time/go/rate"

const (
	ReadonlyScope = gmail_api.GmailReadonlyScope

	// See https://developers.google.com/gmail/api/v1/reference/quota
	quotaUnitsMessagesGet     = 5
	quotaUnitsPerGetProfile   = 2
	quotaUnitsPerHistoryList  = 2
	quotaUnitsPerMessagesList = 1

	quotaUnitsPerSecond = 250
	rateLimitPerSecond  = quotaUnitsPerSecond * 0.8
	rateLimitBurst      = quotaUnitsPerSecond
)

var (
	ErrMessageNotFound = errors.New("gmail message not found")
)

// GmailService provides access to messages stored in Google's GMail
// system.
type GmailService struct {
	service *gmail.Service
	limiter *rate.Limiter
}

func isChat(msg *gmail.Message) bool {
	for _, label := range msg.LabelIds {
		if label == "CHAT" {
			return true
		}
	}
	return false
}

func canonicalError(err error) error {
	if err == nil {
		return err
	}

	// Canonicalize GMail "Not Found" errors to ErrMessageNotFound.
	//
	// GMail sometimes uses internal (phantom) messages in a
	// user's mailbox for somethings (calendar invites?).  These
	// are visible in some Gmail APIs such as Users.history.list.
	// Accordinly, we must handle 404 errors when retrieving them
	// with Users.messages.get.
	//
	// More information:
	// https://issuetracker.google.com/issues/76185867
	// https://issuetracker.google.com/issues/118714982
	// https://issuetracker.google.com/issues/122167541
	//
	// TODO: try including "messageDeleted" in the
	// Users.history.list call, and avoiding calling
	// Users.messages.get for those.  See
	// https://stackoverflow.com/questions/42098553/gmail-api-returns-404-error-when-calling-message-get,
	// which suggests that a history list call can return
	// "messageAdded", "labelAdded" and "labelRemoved" updates for
	// messages that are later deleted.
	//
	// Observation: Programs should be resilient to "Not Found"
	// regardless.  A call to Users.history.list followed by a
	// series of Users.messages.get calls is not an atomic
	// operation.  A message can be deleted at any time for a
	// variety of reasons (e.g. messages in the trash expiring,
	// calls to Users.messages.delete).
	//
	// The error response JSON we're trying to match is this:
	//
	// {
	//   "error": {
	//     "errors": [
	//       {
	//         "domain": "global",
	//         "reason": "notFound",
	//         "message": "Not Found"
	//       }
	//     ],
	//     "code": 404,
	//     "message": "Not Found"
	//   }
	// }
	//
	// TODOD: Handle this more gracefully by generalizing a "not
	// found" error in the MessageStorage interface?
	switch err := errors.Cause(err).(type) {
	case *googleapi.Error:
		if err.Code == http.StatusNotFound {
			for _, item := range err.Errors {
				if item.Reason == "notFound" {
					log.Printf("Warning: message not found...")
					return ErrMessageNotFound
				}
			}
		}
	}
	return err
}

func New(client *http.Client) (*GmailService, error) {
	s, err := gmail.New(client)
	if err != nil {
		return nil, err
	}
	l := rate.NewLimiter(rateLimitPerSecond, rateLimitBurst)
	return &GmailService{service: s, limiter: l}, nil
}

func (s *GmailService) ListAll(ctx context.Context, handler func(message.ID) error) error {
	if err := s.limiter.WaitN(ctx, quotaUnitsPerMessagesList); err != nil {
		return err
	}
	msgs := gmail.NewUsersMessagesService(s.service)
	req := msgs.List("me").Q("-is:chat {in:inbox in:sent}") // XXX "in:all"
	total := 0
	err := req.Pages(ctx, func(page *gmail.ListMessagesResponse) (err error) {
		total += len(page.Messages)
		log.Printf("listed page of Gmail messages; count %d; total so far %d", len(page.Messages), total)
		for _, msg := range page.Messages {
			m := message.ID{PermID: msg.Id, ThreadID: msg.ThreadId}
			if err := handler(m); err != nil {
				return err
			}
		}
		if page.NextPageToken != "" {
			err = s.limiter.WaitN(ctx, quotaUnitsPerMessagesList)
		}
		return
	})
	log.Printf("done listing Gmail messages; total %d", total)
	if err != nil {
		err = errors.Wrap(err, "unable to retrieve all messages")
	}
	return err
}

func (s *GmailService) ListFrom(ctx context.Context, historyID uint64, handler func(message.ID) error) error {
	wait := func() error {
		return s.limiter.WaitN(ctx, quotaUnitsPerHistoryList)
	}
	if err := wait(); err != nil {
		return err
	}

	// TODO: request labelAdded, labelRemoved, messageDeleted too.
	req := gmail.NewUsersHistoryService(s.service).List("me").Context(ctx).HistoryTypes("messageAdded").StartHistoryId(historyID)
	total := 0
	err := req.Pages(ctx, func(page *gmail.ListHistoryResponse) (err error) {
		total += len(page.History)
		log.Printf("listed page of Gmail history; count %d; total so far %d", len(page.History), total)
		for _, h := range page.History {
			// TODO: handle labelAdded, labelRemoved, messageDeleted too.
			for _, added := range h.MessagesAdded {
				m := message.ID{
					PermID:   added.Message.Id,
					ThreadID: added.Message.ThreadId,
				}
				if err := handler(m); err != nil {
					return err
				}
			}
		}
		if page.NextPageToken != "" {
			err = wait()
		}
		return
	})
	log.Printf("done listing Gmail messages; total %d", total)
	if err != nil {
		err = errors.Wrap(err, "unable to retrieve all messages")
	}
	return err
}

func (s *GmailService) GetMessageHeader(ctx context.Context, id string) (*message.Header, error) {
	if err := s.limiter.WaitN(ctx, quotaUnitsMessagesGet); err != nil {
		return nil, err
	}
	msg, err := gmail.NewUsersMessagesService(s.service).Get("me", id).
		Context(ctx).Format("minimal").Do()
	if err == nil && isChat(msg) {
		err = ErrMessageNotFound
	}
	if err != nil {
		err = canonicalError(err)
		return nil, errors.Wrapf(err, "getting message %v from gmail", id)
	}
	m := &message.Header{ID: message.ID{PermID: msg.Id, ThreadID: msg.ThreadId},
		LabelIDs:     msg.LabelIds,
		HistoryID:    msg.HistoryId,
		SizeEstimate: msg.SizeEstimate}
	return m, nil
}

func (s *GmailService) GetMessageFull(ctx context.Context, id string) (*message.Body, error) {
	if err := s.limiter.WaitN(ctx, quotaUnitsMessagesGet); err != nil {
		return nil, err
	}
	msg, err := gmail.NewUsersMessagesService(s.service).Get("me", id).
		Context(ctx).Format("raw").Do()
	if err == nil && isChat(msg) {
		err = ErrMessageNotFound
	}
	if err != nil {
		err = canonicalError(err)
		return nil, errors.Wrapf(err, "getting message %v from gmail", id)
	}
	raw, err := base64.URLEncoding.DecodeString(msg.Raw)
	if err != nil {
		return nil, errors.Wrapf(err, "decoding message %v from gmail", id)
	}
	m := &message.Body{
		Header: message.Header{
			ID:           message.ID{PermID: msg.Id, ThreadID: msg.ThreadId},
			LabelIDs:     msg.LabelIds,
			HistoryID:    msg.HistoryId,
			SizeEstimate: msg.SizeEstimate},
		Raw: string(raw)}
	return m, nil
}

func (s *GmailService) GetProfile(ctx context.Context) (*message.Profile, error) {
	if err := s.limiter.WaitN(ctx, quotaUnitsPerGetProfile); err != nil {
		return nil, err
	}
	u, err := gmail.NewUsersService(s.service).GetProfile("me").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return &message.Profile{
		EmailAddress: u.EmailAddress,
		HistoryID:    u.HistoryId,
	}, nil
}

// func getFormat(minimal bool) string {
// 	if minimal {
// 		return "minimal"
// 	}
// 	return "raw"
// }

// func getMessage(ctx context.Context, limiter *rate.Limiter,
// 	messages *gmail.UsersMessagesService, id string, minimal bool) (msg *gmail.Message, err error) {
// 	msg, err = messages.Get("me", id).Context(ctx).Format(getFormat(minimal)).Do()
// 	if err != nil {
// 		err = errors.Wrapf(err, "getting message %v from gmail", id)
// 	}
// 	return

// 	// tx, err := db.BeginTx(ctx, nil)
// 	// if err != nil {
// 	// 	return errors.Wrap(err, "can't start transaction")
// 	// }
// 	// defer tx.Rollback()

// 	//	for _, label := range m.LabelIds {
// 	//	}

// 	//
// 	// 	err = req.Pages(ctx, func(page *gmail.ListMessagesResponse) error {
// 	// 		return handleListPage(ctx, db, messages, page)
// 	// 	})
// }
