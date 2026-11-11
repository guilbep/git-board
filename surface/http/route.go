package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/obs"
)

// A route is a method: Route decodes its request, calls it, and encodes
// its answer (design.md, The application). The request's fields are
// filled by their tags: `path:"id"` from the path, `query:"filter"` from
// the query string, `header:"If-Match"` from a header, and the body, as
// JSON, into the fields tagged `json`. A body field the request doesn't
// have is refused, named. The answer is JSON, 200, unless it is a Reply.

// Reply is an answer with its status and headers.
type Reply struct {
	Status   int
	ETag     string // a version, quoted here
	Location string
	Body     any // nil: no body
}

// maxBody is the most a request's JSON may weigh.
const maxBody = 16 << 20

// Route registers f at pattern ("POST /api/issues/{id}/comments").
func Route[Req, Res any](mux *http.ServeMux, pattern string, f func(context.Context, Req) (Res, error)) {
	dec := decoderOf(reflect.TypeFor[Req]())
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		obs.Name(r.Context(), pattern)
		var req Req
		if err := dec.decode(r, reflect.ValueOf(&req).Elem()); err != nil {
			writeError(w, r, logOf(r.Context()), err)
			return
		}
		res, err := f(r.Context(), req)
		if err != nil {
			writeError(w, r, logOf(r.Context()), err)
			return
		}
		reply(w, r, any(res))
	})
}

func reply(w http.ResponseWriter, r *http.Request, res any) {
	rep, ok := res.(Reply)
	if !ok {
		writeJSON(w, r, http.StatusOK, res)
		return
	}
	if rep.ETag != "" {
		w.Header().Set("ETag", strconv.Quote(rep.ETag))
	}
	if rep.Location != "" {
		w.Header().Set("Location", rep.Location)
	}
	status := cmpOr(rep.Status, http.StatusOK)
	if rep.Body == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, r, status, rep.Body)
}

func cmpOr(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// decoder fills one request type.
type decoder struct {
	fields []tagged
	body   bool // some field is the body's
}

type tagged struct {
	index      int
	from, name string // path, query or header
}

func decoderOf(t reflect.Type) decoder {
	var d decoder
	if t.Kind() != reflect.Struct {
		return d
	}
	for i := range t.NumField() {
		f := t.Field(i)
		for _, from := range []string{"path", "query", "header"} {
			if name, ok := f.Tag.Lookup(from); ok {
				d.fields = append(d.fields, tagged{i, from, name})
			}
		}
		if tag, ok := f.Tag.Lookup("json"); ok && tag != "-" {
			d.body = true
		}
	}
	return d
}

func (d decoder) decode(r *http.Request, v reflect.Value) error {
	if d.body && r.Method != http.MethodGet && r.Method != http.MethodHead {
		data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
		if err != nil {
			return invalid("", fmt.Errorf("reading the request: %w", err))
		}
		if len(bytes.TrimSpace(data)) > 0 {
			jd := json.NewDecoder(bytes.NewReader(data))
			jd.DisallowUnknownFields()
			if err := jd.Decode(v.Addr().Interface()); err != nil {
				return invalid(jsonField(err), fmt.Errorf("the request's JSON: %w", err))
			}
		}
	}
	for _, f := range d.fields {
		var raw string
		var set bool
		switch f.from {
		case "path":
			raw = r.PathValue(f.name)
			set = true
		case "query":
			q := r.URL.Query()
			if vs, ok := q[f.name]; ok {
				if fv := v.Field(f.index); fv.Kind() == reflect.Slice {
					fv.Set(reflect.ValueOf(vs))
					continue
				}
				raw, set = vs[0], true
			}
		case "header":
			raw = r.Header.Get(f.name)
			set = raw != ""
		}
		if !set {
			continue
		}
		if err := setString(v.Field(f.index), raw); err != nil {
			return invalid(f.name, fmt.Errorf("%s %q: %w", f.name, raw, err))
		}
	}
	return nil
}

// setString sets a field from its text: a string, a number, a bool, or
// a *string (present, even empty).
func setString(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Int:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return errors.New("not a number")
		}
		fv.SetInt(int64(n))
	case reflect.Bool:
		b, err := strconv.ParseBool(cmpString(raw, "true"))
		if err != nil {
			return errors.New("true or false")
		}
		fv.SetBool(b)
	case reflect.Pointer:
		p := reflect.New(fv.Type().Elem())
		if err := setString(p.Elem(), raw); err != nil {
			return err
		}
		fv.Set(p)
	default:
		return fmt.Errorf("can't be set from text (%s)", fv.Kind())
	}
	return nil
}

func cmpString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// jsonField is the field a JSON error is about, when it says.
func jsonField(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return te.Field
	}
	if f, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return strings.Trim(f, `"`)
	}
	return ""
}

// version is an If-Match or If-None-Match header's version: the quotes
// and a weak prefix taken off; "" for none, or *.
func version(header string) string {
	h := strings.TrimPrefix(strings.TrimSpace(header), "W/")
	if h == "*" {
		return ""
	}
	if s, err := strconv.Unquote(h); err == nil {
		return s
	}
	return h
}
