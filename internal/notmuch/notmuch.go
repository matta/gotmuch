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

package notmuch

import (
	"context"
	"errors"
	"hash/fnv"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"marmstrong/gotmuch/internal/message"
)

const (
	dirFileMode     = 0700 // TODO: make configurable?
	messageFileMode = 0600 // TODO: make configurable?

	pathFarm16 = "abcdefghijklmnop"
)

type Service struct {
	// Path to the directory we're writing files to within the
	// notmuch database.  Equivalent to; `notmuch config get
	// database.path` and appending the subdir.
	path string
}

type path struct {
	root string
	dirs []string
	base string
}

func (p path) Join() string {
	parts := make([]string, 1, len(p.dirs)+2)
	parts[0] = p.root
	parts = append(parts, p.dirs...)
	parts = append(parts, p.base)
	return filepath.Join(parts...)
}

func New() (*Service, error) {
	// TODO: make the notmuch binary name configurable.
	out, err := exec.Command("notmuch", "config", "get", "database.path").Output()
	if err != nil {
		return nil, err
	}
	s := &Service{}
	// TODO: make "gotmuch" configurable.
	// TODO: include the scope (login) in the base path here.
	s.path = filepath.Join(strings.TrimSpace(string(out)), "gotmuch")

	err = mkdirfarm(s.path, 2)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) HaveMessage(id string) bool {
	_, err := os.Stat(s.makePath(id).Join())
	return err == nil
}

func (s *Service) Insert(ctx context.Context, msg *message.Body) error {
	if msg.PermID == "" {
		return errors.New("message has no ID")
	}
	if msg.Raw == "" {
		return errors.New("message has no content")
	}
	path := s.makePath(msg.PermID)

	// Replace all \r\n with \n.  The GMail API delivers messages
	// in this form because it is mandated by RFC 822 and
	// successors.
	//
	// TODO: sanitize further?  Look for expected headers.  Ensure
	// trailing newline.
	//
	// TODO: this can be optimized.
	// E.g. https://godoc.org/golang.org/x/text/transform#SpanningTransformer
	// or the equivalent hand rolled.
	raw := strings.ReplaceAll(msg.Raw, "\r\n", "\n")
	return ioutil.WriteFile(path.Join(), []byte(raw), messageFileMode)
}

// basename holds the fields encoded into the basename portion of the
// file name of messages delivered to notuch.
type basename struct {
	// A unique string designating the scope under which the
	// permID is both unique and permanent.  In the case of GMail,
	// the user's Google login is used.
	scope string

	// A unique string identifying the message.  In the case of
	// GMail this is the GMail API's Users.messages resource "id"
	// field, which within this program is also stored in
	// message.ID.PermId.
	permID string
}

// Return the specified string with characters that should not appear
// in a notmuch Maildir filename escaped.
func escape(s string) string {
	hexCount := 0
	for i := 0; i < len(s); i++ {
		if shouldEscape(s[i]) {
			hexCount++
		}
	}

	if hexCount == 0 {
		return s
	}

	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case shouldEscape(c):
			t[j] = '='
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		default:
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

// Return true if the specified character should be escaped when
// appearing in a notmuch Maildir filename.
//
// The encoding uses the underscore to designate the next two
// characters as a hex encoded byte.
//
// Based on the following IEEE specification, with the revision that
// the all punctuation is removed, leaving only alphanumeric
// characters.  See:
//
// The Open Group Base Specifications Issue 7, 2018 edition, IEEE Std
// 1003.1-2017 (Revision of IEEE Std 1003.1-2008).
// 3.282 Portable Filename Character Set
func shouldEscape(c byte) bool {
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return false
	}

	// Everything else must be escaped.
	return true
}

// Encode returns the basename encoded in a filename (and Maildir)
// safe form.
//
// The encoding URL arg encodes each field, and then base64url encodes
// the result, prefixed with "gotmuch-1-", as a distinguisher followed
// by an encoding version.
func (b basename) encode() string {
	var sb strings.Builder
	const prefix = "gotmuch-1-"
	sb.Grow(len(prefix) + len(b.scope) + len(b.permID) + 1)
	sb.WriteString(prefix)
	sb.WriteString(escape(b.scope))
	sb.WriteRune('-')
	sb.WriteString(escape(b.permID))
	return sb.String()
}

func mkdir(dir string) error {
	if err := os.Mkdir(dir, dirFileMode); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

func mkdirfarm(path string, depth int) error {
	if err := mkdir(path); err != nil {
		return err
	}
	if depth == 0 {
		return nil
	}

	for i := 0; i < len(pathFarm16); i++ {
		path := filepath.Join(path, pathFarm16[i:i+1])
		if err := mkdirfarm(path, depth-1); err != nil {
			return err
		}
	}
	return nil
}

func fingerprint(b []byte) uint32 {
	hash := fnv.New32a()
	hash.Write(b)
	return hash.Sum32()
}

func pathParts(id string) []string {
	fp := fingerprint([]byte(id))
	nibble1 := fp & 0xf
	nibble2 := (fp >> 4) & 0xf
	return []string{pathFarm16[nibble1 : nibble1+1], pathFarm16[nibble2 : nibble2+1]}
}

func (s *Service) makePath(id string) path {
	return path{
		root: s.path,
		dirs: pathParts(id),
		// TODO: use a real "scope" here.
		base: basename{scope: "xxx", permID: id}.encode(),
	}
}
