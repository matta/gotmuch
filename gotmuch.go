// The gotmuch command is a utility that DESCRIBE ME.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google3/base/go/flag"
	"google3/base/go/google"
	"google3/base/go/log"
	"google3/experimental/users/marmstrong/gotmuch/gmail"
	"google3/experimental/users/marmstrong/gotmuch/gmailhttp"
	"google3/experimental/users/marmstrong/gotmuch/homedir"
	"google3/experimental/users/marmstrong/gotmuch/notmuch"
	"google3/experimental/users/marmstrong/gotmuch/persist"
	"google3/experimental/users/marmstrong/gotmuch/sync"
	"google3/experimental/users/marmstrong/gotmuch/tracehttp"
	"google3/third_party/golang/errors/errors"
	_ "google3/third_party/golang/sqlite3/sqlite3"
)

var (
	flagTrace = flag.Bool("T", false, "request debug tracing")
)

func run() error {
	nm, err := notmuch.New()
	if err != nil {
		return errors.Wrap(err, "Unable to initialize notmuch")
	}

	ctx := context.Background()
	db, err := persist.OpenDB(ctx, filepath.Join(homedir.Get(), ".gotmuch.db"))
	if err != nil {
		return errors.Wrap(err, "Unable to initialize database")
	}
	defer db.Close()

	s, err := gmail.New(gmailhttp.New())
	if err != nil {
		return errors.Wrap(err, "Unable to initialize GMail")
	}

	err = sync.CatchUp(ctx, s, db, nm)
	if err != nil {
		return errors.Wrap(err, "Unable to synchronize")
	}
	return nil
}

func main() {
	google.Init()

	if *flagTrace {
		tracehttp.WrapDefaultTransport()
	}

	if err := run(); err != nil {
		log.Exitf("Failed: %v\n", err)
	}
	fmt.Print("Success!\n")
	os.Exit(0)
}
