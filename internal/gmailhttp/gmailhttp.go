/*
Pakage gmailhttp implements an HTTP client for gmail.

Status: supports access from Google's internal corporate networks,
from a corporate desktop, for the current user.

OAuth2.0 tokens are acquired by running an external program.  The
program shoud behave identically to the one used by
https://github.com/google/oauth2l (see
https://github.com/google/oauth2l/blob/master/util/sso.go).

The required API Key, required by Google's corporate GMail setup,
is hard coded in the source.  TODO: don't do that.  ;-)

Note: this program cannot use the Go API provided by
https://github.com/google/oauth2l for three primary reasons:

1) no support for SSO auth from the oauth2l Go API.  Support is there
   only from the package's command line "oauth2l" program.

1) no direct support for using OAuth2.0 with API Keys.  If an API Key
   is set in the oauth2l.Config the code attempts a non-OAuth
   authentication method.

2) no support for refreshing tokens on expiry.  The http.Client
   returned by the library never refreshes tokens, which is broken for
   long lived clients (and possibly short lived dones too, if the
   token happens to have an expire time in the near future).

BUGS:

This package code hard codes a tokens' expire time at one hour, since
the API supported by the oauth2l SSO program does not provide the
tokens' actual expire time.  It isn't clear that the
golang.org/x/oauth2 approach to token expiry, the token.Valid()
method, encourages well designed software anyway.  OAuth 2.0 clients
should be designed to gracefully handle expired token responses from
the server at any time.  The client's notion of token expiry should be
at most an optimization that prevents unecessary network round trips,
but golang.org/x/oauth2 seems to be designed on the assumption that
the client has perfect knowledge of when a token will expire (which is
impossible).

*/

package gmailhttp

import (
	"bytes"
	"net/http"
	"os/exec"
	"os/user"
	"time"

	"github.com/matta/gotmuch/internal/gmail"
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

// username returns the current user name or an error.
func username() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

// New returns a new HTTP client capable of using the GMail API.
func New() (*http.Client, error) {
	// TODO: do not hard code the user.
	user, err := username()
	if err != nil {
		return nil, err
	}

	src := &ssoTokenSource{
		// TODO: do not hard code the sso command path.
		sso: "/google/data/ro/teams/oneplatform/sso",
		// TODO: do not hard code "@google.com".
		user:  user + "@google.com",
		scope: gmail.ReadonlyScope,
	}

	// This API key is generated from the Google Developer
	// Console: API & Auth -> APIs -> Credentials -> Add
	// Credentials.  Type=Server.  No IP restrictions (probably
	// unwise).
	//
	// TODO: do not hard code the API Key.
	const apiKey = "AIzaSyC5jDn2OKqDbJhObCasuNg8QYoaxJhmWiI"

	trans := &oauth2.Transport{
		Source: oauth2.ReuseTokenSource(nil, src),
		Base:   &transport.APIKey{Key: apiKey},
	}

	return &http.Client{Transport: trans}, nil
}
