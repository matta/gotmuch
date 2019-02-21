// The gotmuch command is a utility that DESCRIBE ME.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/matta/gotmuch/internal/gmail"
	"github.com/matta/gotmuch/internal/gmailhttp"
	"github.com/matta/gotmuch/internal/homedir"
	"github.com/matta/gotmuch/internal/notmuch"
	"github.com/matta/gotmuch/internal/persist"
	"github.com/matta/gotmuch/internal/sync"
	"github.com/matta/gotmuch/internal/tracehttp"
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
	return nil
}

func main() {
	if *flagTrace {
		tracehttp.WrapDefaultTransport()
	}

	if err := run(); err != nil {
		log.Fatalf("Failed: %v\n", err)
	}
	fmt.Print("Success!\n")
	os.Exit(0)
}
