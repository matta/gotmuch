/*
Pakage gmailhttp implements an HTTP client for gmail.

OAuth2.0 tokens are acquired by running an external program.  The
program shoud behave identically to the one used by
https://github.com/google/oauth2l (see
https://github.com/google/oauth2l/blob/abeb08f278e7973101d881b5d962055bf52f3950/util/sso.go#L24).

BUGS:

Token expiry may not be be handled properly.

*/

package gmailhttp

import (
	"bytes"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"strings"
	"time"

	"marmstrong/gotmuch/internal/gmail"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi/transport"
)

// ssoTokenSource encodes the information required to run an external
// program to retrieve an OAuth 2.0 bearer token for a given user and
// set of scopes.
type ssoTokenSource struct {
	// The sso command name.
	sso string

	// The user name to authenticate.
	user string

	// The scope (space separated) to authenticate.
	//
	// TODO: make this a slice
	scope string
}

// Token returns a new token for the specified user and scopes by
// executing the specified external program.  Satisfies
// oauth2.TokenSource.
func (s *ssoTokenSource) Token() (*oauth2.Token, error) {
	cmd := exec.Command(s.sso, s.user, s.scope)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	accessToken := out.String()

	return &oauth2.Token{
		AccessToken: accessToken,
		// TODO: figure out a good solution for token
		// expiration.  The pair of oauth2.ReuseTokenSource
		// and oauth2.Transport is insufficient because there
		// is no code to handle token invalidation done by the
		// server.  To mitigate that we re-fetch a token every
		// 5 minutes, but this will be insufficient if the
		// token is ever cached.
		Expiry: time.Now().Add(time.Minute * 5),
	}, nil
}

// New returns a new HTTP client capable of using the GMail API.
func New() (*http.Client, error) {
	user, ok := os.LookupEnv("GOTMUCH_USER")
	if !ok {
		return nil, errors.New("GOTMUCH_USER environment must be set")
	}
	addr, err := mail.ParseAddress(user)
	if err != nil {
		return nil, errors.Wrapf(err, "GOTMUCH_USER fails to parse as an email address: %q", user)
	}
	login := addr.Address
	if !strings.ContainsRune(login, '@') {
		return nil, fmt.Errorf("GOTMUCH_USER must contain a hostname: %q", login)
	}

	sso, ok := os.LookupEnv("GOTMUCH_SSO")
	if !ok {
		return nil, errors.New("GOTMUCH_SSO environment variable must be set")
	}

	src := &ssoTokenSource{
		sso:   sso,
		user:  login,
		scope: gmail.ReadonlyScope,
	}

	trans := &oauth2.Transport{Source: oauth2.ReuseTokenSource(nil, src)}

	apiKey, ok := os.LookupEnv("GOTMUCH_API_KEY")
	if ok {
		// This API key is generated from the Google Developer
		// Console: API & Auth -> APIs -> Credentials -> Add
		// Credentials.  Type=Server.
		trans.Base = &transport.APIKey{Key: apiKey}
	}

	return &http.Client{Transport: trans}, nil
}
