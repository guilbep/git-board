package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
)

// apiError is the one error shape (docs/rewrite/api.md): a code of a
// fixed list, a sentence, and the request's field it is about.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// fieldError is an error about one field of the request: the adapter
// knows which field fed the call.
type fieldError struct {
	field string
	err   error
}

func (e *fieldError) Error() string { return e.err.Error() }
func (e *fieldError) Unwrap() error { return e.err }

// invalid is a request the server can't read: a usage error about
// field.
func invalid(field string, err error) error {
	return &fieldError{field, &app.Error{Class: app.Usage, Err: err}}
}

// on names the field err is about, unless it names one already.
func on(field string, err error) error {
	var fe *fieldError
	if err == nil || errors.As(err, &fe) {
		return err
	}
	// a filter that can't be read is the request's, whatever it wraps
	if fe := (*issues.FilterError)(nil); errors.As(err, &fe) && app.Classify(err) == app.Internal {
		err = &app.Error{Class: app.Usage, Err: err}
	}
	return &fieldError{field, err}
}

// codeOf is err's code and status: a few the server makes itself (a
// body that isn't JSON), else app's table.
func codeOf(err error) (string, int) {
	var c *coded
	if errors.As(err, &c) {
		return c.code, c.status
	}
	return app.Code(err), app.HTTPStatus(err)
}

// coded is an error the server answers with its own code and status:
// not-json, not-found for a route, forbidden.
type coded struct {
	code   string
	status int
	err    error
}

func (e *coded) Error() string { return e.err.Error() }
func (e *coded) Unwrap() error { return e.err }

// writeError answers err in the one shape, and logs it: an internal
// error at error, with its code.
func writeError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	code, status := codeOf(err)
	body := apiError{Code: code, Message: app.NoCredentials(err.Error())}
	var fe *fieldError
	if errors.As(err, &fe) {
		body.Field = fe.field
	}
	if status >= 500 && log != nil {
		log.LogAttrs(r.Context(), slog.LevelError, "request failed", slog.String("method", r.Method),
			slog.String("path", r.URL.Path), slog.String("code", code), slog.String("error", body.Message))
	}
	if rec, ok := w.(*recorder); ok {
		rec.code = code
	}
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
