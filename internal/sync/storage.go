// Package main holds the whole shebang.
package sync

// This file provides the common data objects used by the rest of the
// program.

import (
	"context"

	"marmstrong/gotmuch/internal/message"
)

// MessageLister lists all message identifiers from a message storage
// system.
type MessageLister interface {
	ListAll(ctx context.Context, handler func(*message.ID) error) error
	ListFrom(ctx context.Context, historyId uint64, handler func(*message.ID) error) error
}

// MessageMetaGetter gets per message metadata from message storage
// system.
type MessageMetaGetter interface {
	GetMessageMeta(ctx context.Context, id string) (*message.Header, error)
	GetMessageFull(ctx context.Context, id string) (*message.Body, error)
}

// MessageProfiler gets per account metadata from a message storage
// system.
type MessageProfiler interface {
	GetProfile(ctx context.Context) (*message.Profile, error)
}

// MessageStorage provides all possible actions available to deal with
// message storage.
type MessageStorage interface {
	MessageLister
	MessageMetaGetter
	MessageProfiler
}
