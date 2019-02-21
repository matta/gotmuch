// Package gmail TODO(marmstrong): writeme
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
)

// "google3/util/time/go/rate"

const (
	ReadonlyScope = gmail_api.GmailReadonlyScope

	// See https://developers.google.com/gmail/api/v1/reference/quota
	gmailQuotaUnitsPerSecond       = 250
	gmailQuotaUnitsPerMessagesList = 1
	gmailQuotaUnitsPerHistoryList  = 2
)

// GmailService provides access to messages stored in Google's GMail
// system.
type GmailService struct {
	service *gmail.Service
	limiter *rate.Limiter
}

// NewGmailService TODO(marmstrong): writeme
func New(client *http.Client) (*GmailService, error) {
	s, err := gmail.New(client)
	if err != nil {
		return nil, err
	}
	l := rate.NewLimiter(gmailQuotaUnitsPerSecond/2.0, gmailQuotaUnitsPerSecond)
	return &GmailService{service: s, limiter: l}, nil
}

// ListAll TODO(marmstrong): writeme
func (s *GmailService) ListAll(ctx context.Context, handler func(*message.ID) error) error {
	if err := s.limiter.WaitN(ctx, gmailQuotaUnitsPerMessagesList); err != nil {
		return err
	}
	msgs := gmail.NewUsersMessagesService(s.service)
	req := msgs.List("me").Q("{in:inbox is:sent}") // XXX "in:all"
	total := 0
	err := req.Pages(ctx, func(page *gmail.ListMessagesResponse) (err error) {
		total += len(page.Messages)
		log.Printf("listed page of Gmail messages; count %d; total so far %d", len(page.Messages), total)
		for _, msg := range page.Messages {
			m := &message.ID{PermID: msg.Id, ThreadID: msg.ThreadId}
			if err := handler(m); err != nil {
				return err
			}
		}
		if page.NextPageToken != "" {
			err = s.limiter.WaitN(ctx, gmailQuotaUnitsPerMessagesList)
		}
		return
	})
	log.Printf("done listing Gmail messages; total %d", total)
	if err != nil {
		err = errors.Wrap(err, "unable to retrieve all messages")
	}
	return err
}

func (s *GmailService) ListFrom(ctx context.Context, historyID uint64, handler func(*message.ID) error) error {
	wait := func() error {
		return s.limiter.WaitN(ctx, gmailQuotaUnitsPerHistoryList)
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
				m := &message.ID{
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

func (s *GmailService) GetMessageMeta(ctx context.Context, id string) (*message.Header, error) {
	msg, err := gmail.NewUsersMessagesService(s.service).Get("me", id).
		Context(ctx).Format("minimal").Do()
	if err != nil {
		return nil, errors.Wrapf(err, "getting message %v from gmail", id)
	}
	m := &message.Header{ID: message.ID{PermID: msg.Id, ThreadID: msg.ThreadId},
		LabelIds:     msg.LabelIds,
		HistoryID:    msg.HistoryId,
		SizeEstimate: msg.SizeEstimate}
	return m, nil
}

func (s *GmailService) GetMessageFull(ctx context.Context, id string) (*message.Body, error) {
	msg, err := gmail.NewUsersMessagesService(s.service).Get("me", id).
		Context(ctx).Format("raw").Do()
	if err != nil {
		return nil, errors.Wrapf(err, "getting message %v from gmail", id)
	}
	raw, err := base64.URLEncoding.DecodeString(msg.Raw)
	if err != nil {
		return nil, errors.Wrapf(err, "decoding message %v from gmail", id)
	}
	m := &message.Body{
		Header: message.Header{
			ID:           message.ID{PermID: msg.Id, ThreadID: msg.ThreadId},
			LabelIds:     msg.LabelIds,
			HistoryID:    msg.HistoryId,
			SizeEstimate: msg.SizeEstimate},
		Raw: string(raw)}
	return m, nil
}

func (s *GmailService) GetProfile(ctx context.Context) (*message.Profile, error) {
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
