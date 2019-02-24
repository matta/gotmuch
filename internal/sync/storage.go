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
	GetMessageHeader(ctx context.Context, id string) (*message.Header, error)
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
