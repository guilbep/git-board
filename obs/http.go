package obs

import (
	"net/http"
)

// Transport traces each request rt makes: its method, host, path and
// status, a client span of the caller's context. It sends no traceparent:
// a forge is no part of this trace. rt nil is http.DefaultTransport.
func Transport(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return transport{rt}
}

type transport struct{ rt http.RoundTripper }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !On() {
		return t.rt.RoundTrip(r)
	}
	ctx, end := StartClient(r.Context(), r.Method+" "+r.URL.Host, String("http.method", r.Method),
		String("server.address", r.URL.Host), String("url.path", r.URL.Path))
	res, err := t.rt.RoundTrip(r.WithContext(ctx))
	if err == nil {
		Set(ctx, Int("http.status_code", int64(res.StatusCode)))
		if res.StatusCode >= 400 {
			end(httpError(res.Status))
			return res, err
		}
	}
	end(err)
	return res, err
}

type httpError string

func (e httpError) Error() string { return string(e) }
