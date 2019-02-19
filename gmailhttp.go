package gmailhttp

import (
	"net/http"

	"google3/experimental/users/marmstrong/gotmuch/gmail"
	"google3/security/corplogin/go/sso_goauth2"
	"google3/third_party/golang/google_api/googleapi/transport/transport"
)

func New() *http.Client {
	// http://g3doc/company/teams/sso/howto/oauth_from_loas
	var ssoTransport = sso_goauth2.TransportForCurrentUser([]string{gmail.ReadonlyScope})

	// This API key is generated from the Google Developer Console: API
	// & Auth -> APIs -> Credentials -> Add Credentials.  Type=Server.
	// No IP restrictions (probably unwise).
	apiKey := "AIzaSyC5jDn2OKqDbJhObCasuNg8QYoaxJhmWiI"

	apiTransport := transport.APIKey{
		Key:       apiKey,
		Transport: ssoTransport}
	return &http.Client{Transport: &apiTransport}
}
