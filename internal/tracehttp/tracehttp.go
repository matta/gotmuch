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
