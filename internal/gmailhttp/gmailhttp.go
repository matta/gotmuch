package gmailhttp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"os/user"
	"time"

	"github.com/matta/gotmuch/internal/gmail"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi/transport"
)

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

// Satisfy oauth2.TokenSource.
func (s *ssoTokenSource) Token() (*oauth2.Token, error) {
	cmd := exec.Command(s.sso, s.user, s.scope)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	accessToken := out.String()
	fmt.Printf("got sso token %q\n", accessToken)

	return &oauth2.Token{
		AccessToken: accessToken,
		Expiry:      time.Now().Add(time.Minute * 5),
	}, nil
}

func username() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func New(ctx context.Context) (*http.Client, error) {
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
