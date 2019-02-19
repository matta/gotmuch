package tracehttp

import (
	"fmt"
	"net/http"
	"net/http/httputil"
)

// TraceTransport is an http.RoundTripper that prints the request and
// response to stdout while delegating the real work to another
// http.RoundTripper.
type traceTransport struct {
	delegate http.RoundTripper
}

// RoundTrip prints a dump of the request and response while delegating the
// round trip to the delegate.
func (t *traceTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	dump, dumpErr := httputil.DumpRequest(req, true)
	if dumpErr == nil {
		fmt.Println(string(dump))
	}
	resp, err = t.delegate.RoundTrip(req)
	if err == nil {
		dump, dumpErr = httputil.DumpResponse(resp, true)
		if dumpErr == nil {
			fmt.Println(string(dump))
		}
	}
	return resp, err
}

func Wrap(d http.RoundTripper) http.RoundTripper {
	return &traceTransport{d}
}

// Inject a TraceTransport into http.DefaultTransport
func WrapDefaultTransport() {
	http.DefaultTransport = Wrap(http.DefaultTransport)
}
