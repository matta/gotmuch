package message

// This file provides the common data objects used by the rest of the
// program.

// ID defines the properties that uniquely identify a message.
type ID struct {
	// The permanent and unique ID of a message in a storage
	// system.
	PermID string

	// The permanent and unique ID of a thread associated with the
	// message.  May be empty in storage systems that do not
	// support this concept.
	ThreadID string
}

// Header defines the metadata associated with a message.
type Header struct {
	// The message's permanent unique identifiers.
	ID

	// The current set of label identifiers associated with the
	// message.  These identifiers are not the user visible label
	// names!
	LabelIds []string

	// An estimated size of the message (bytes).
	SizeEstimate int64

	// An opque identifier naming the snapshot in time at which
	// this record was taken.  Values need not be monotonic.
	HistoryID uint64
}

// Body defines a complete message, including the message body.
type Body struct {
	Header

	// The entire email message in an RFC 2822 formatted string.
	Raw string
}

// Profile defines per-account information in a message mailbox.
type Profile struct {
	EmailAddress string

	// The ID of the mailbox's current history record.
	HistoryID uint64
}
