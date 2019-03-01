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

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"marmstrong/gotmuch/internal/gmail"
	"marmstrong/gotmuch/internal/gmailhttp"
	"marmstrong/gotmuch/internal/homedir"
	"marmstrong/gotmuch/internal/notmuch"
	"marmstrong/gotmuch/internal/persist"
	"marmstrong/gotmuch/internal/sync"
	"marmstrong/gotmuch/internal/tracehttp"

	"github.com/pkg/errors"

	_ "github.com/mattn/go-sqlite3"
)

var (
	flagTrace = flag.Bool("T", false, "request debug tracing")
)

func run() error {
	nm, err := notmuch.New()
	if err != nil {
		return errors.Wrap(err, "unable to initialize notmuch")
	}

	ctx := context.Background()
	db, err := persist.Open(ctx, filepath.Join(homedir.Get(), ".gotmuch.db"))
	if err != nil {
		return errors.Wrap(err, "unable to initialize database")
	}
	defer db.Close()

	http, err := gmailhttp.New()
	if err != nil {
		return errors.Wrap(err, "unable to initialize GMail HTTP client")
	}

	s, err := gmail.New(http)
	if err != nil {
		return errors.Wrap(err, "unable to initialize GMail")
	}

	err = sync.Sync(ctx, s, db, nm)
	if err != nil {
		return errors.Wrap(err, "unable to synchronize")
	}

	err = db.Close()
	if err != nil {
		return errors.Wrap(err, "unable to close db")
	}
	return nil
}

func main() {
	flag.Parse()
	if *flagTrace {
		tracehttp.WrapDefaultTransport()
	}

	if err := run(); err != nil {
		log.Fatalf("Failed: %v\n", err)
	}
	log.Print("Success!\n")
	os.Exit(0)
}
