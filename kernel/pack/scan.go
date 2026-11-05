package pack

import (
	"encoding/json"
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// A first build decodes every pack of a board: 100 MB of JSON at 20,000
// issues. encoding/json scans each byte twice (it checks the whole input,
// then decodes), and a kind's fold decodes each operation's data again, so
// a byte was scanned four times. This file reads the shapes git-board
// writes in one pass: objects of plain keys, strings, integers, arrays of
// strings, objects of strings. Anything else, or anything it can't vouch
// for, is left to encoding/json, so a pack decodes exactly as v1 decodes
// it: keys in any case, the last of two equal keys, invalid UTF-8 as
// U+FFFD, the same errors.

// errSlow says the fast path can't vouch for the input.
type errSlow struct{}

func (errSlow) Error() string { return "pack: not read by the fast path" }

var slow error = errSlow{}

// scanner reads one JSON text.
type scanner struct {
	b []byte
	i int
}

func (s *scanner) space() {
	for s.i < len(s.b) {
		switch s.b[s.i] {
		case ' ', '\n', '\t', '\r':
			s.i++
		default:
			return
		}
	}
}

// next is the next byte after spaces, 0 at the end (a 0 byte is no
// JSON's: whoever reads it fails).
func (s *scanner) next() byte {
	s.space()
	if s.i >= len(s.b) {
		return 0
	}
	return s.b[s.i]
}

// str reads a string, unescaped as encoding/json unescapes one: invalid
// UTF-8 and a lone surrogate become U+FFFD.
func (s *scanner) str() (string, error) {
	if s.next() != '"' {
		return "", slow
	}
	start := s.i + 1
	plain := true
	for j := start; j < len(s.b); j++ {
		c := s.b[j]
		if !special[c] {
			continue
		}
		switch {
		case c == '"':
			s.i = j + 1
			if plain {
				return string(s.b[start:j]), nil
			}
			return unquote(s.b[start:j])
		case c == '\\':
			plain = false
			j++
		case c < 0x20:
			return "", slow // invalid in a string: encoding/json says how
		default: // c >= utf8.RuneSelf
			if r, size := utf8.DecodeRune(s.b[j:]); r == utf8.RuneError && size <= 1 {
				plain = false
			} else {
				j += size - 1
			}
		}
	}
	return "", slow
}

// skipStr reads past a string no field takes, checking it as
// encoding/json's scanner does (escapes, control characters), building
// nothing.
func (s *scanner) skipStr() error {
	for j := s.i + 1; j < len(s.b); j++ {
		switch c := s.b[j]; {
		case c == '"':
			s.i = j + 1
			return nil
		case c < 0x20:
			return slow
		case c == '\\':
			if j+1 >= len(s.b) {
				return slow
			}
			switch s.b[j+1] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				j++
			case 'u':
				if u4(s.b[j:]) < 0 {
					return slow
				}
				j += 5
			default:
				return slow
			}
		}
	}
	return slow
}

// special are the bytes a string's scan stops at.
var special = func() (t [256]bool) {
	for i := range t {
		t[i] = i == '"' || i == '\\' || i < 0x20 || i >= utf8.RuneSelf
	}
	return t
}()

// unquote is a string's text, as encoding/json's unquoteBytes reads it.
func unquote(s []byte) (string, error) {
	b := make([]byte, 0, len(s)+utf8.UTFMax)
	for r := 0; r < len(s); {
		switch c := s[r]; {
		case c == '\\':
			if r+1 >= len(s) {
				return "", slow
			}
			switch e := s[r+1]; e {
			case '"', '\\', '/':
				b = append(b, e)
				r += 2
			case 'b':
				b, r = append(b, '\b'), r+2
			case 'f':
				b, r = append(b, '\f'), r+2
			case 'n':
				b, r = append(b, '\n'), r+2
			case 'r':
				b, r = append(b, '\r'), r+2
			case 't':
				b, r = append(b, '\t'), r+2
			case 'u':
				rr := u4(s[r:])
				if rr < 0 {
					return "", slow
				}
				r += 6
				if utf16.IsSurrogate(rr) {
					if dec := utf16.DecodeRune(rr, u4(s[r:])); dec != unicode.ReplacementChar {
						rr = dec
						r += 6
					} else {
						rr = unicode.ReplacementChar
					}
				}
				b = utf8.AppendRune(b, rr)
			default:
				return "", slow
			}
		case c < 0x20:
			return "", slow
		case c < utf8.RuneSelf:
			n := r + 1 // a run of plain bytes, appended at once
			for n < len(s) && !special[s[n]] {
				n++
			}
			b = append(b, s[r:n]...)
			r = n
		default:
			rr, size := utf8.DecodeRune(s[r:])
			r += size
			b = utf8.AppendRune(b, rr)
		}
	}
	return string(b), nil
}

// u4 is the code point of a \uXXXX, -1 if s doesn't start with one.
func u4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case '0' <= c && c <= '9':
			c -= '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}
	return r
}

// key reads an object's key: a plain ASCII one, the case encoding/json
// folds simply.
func (s *scanner) key() (string, error) {
	k, err := s.str()
	if err != nil {
		return "", err
	}
	for i := 0; i < len(k); i++ {
		if k[i] >= utf8.RuneSelf {
			return "", slow
		}
	}
	if s.next() != ':' {
		return "", slow
	}
	s.i++
	return k, nil
}

// object reads an object, calling field for each key; field reads its
// value. A key twice fails it: encoding/json merges some values.
func (s *scanner) object(field func(key string) error) error {
	if s.next() != '{' {
		return slow
	}
	s.i++
	if s.next() == '}' {
		s.i++
		return nil
	}
	var seen [16]string
	n := 0
	for {
		k, err := s.key()
		if err != nil {
			return err
		}
		for _, x := range seen[:min(n, len(seen))] {
			if equalFold(x, k) {
				return slow
			}
		}
		if n >= len(seen) {
			return slow
		}
		seen[n] = k
		n++
		if err := field(k); err != nil {
			return err
		}
		switch s.next() {
		case ',':
			s.i++
		case '}':
			s.i++
			return nil
		default:
			return slow
		}
	}
}

// value skips any value, checking it as JSON: an unknown field's.
func (s *scanner) value() error {
	switch c := s.next(); {
	case c == '"':
		return s.skipStr()
	case c == '{':
		return s.object(func(string) error { return s.value() })
	case c == '[':
		return s.array(func() error { return s.value() })
	case c == 't':
		return s.lit("true")
	case c == 'f':
		return s.lit("false")
	case c == 'n':
		return s.lit("null")
	default:
		_, err := s.number()
		return err
	}
}

func (s *scanner) lit(w string) error {
	if len(s.b)-s.i < len(w) || string(s.b[s.i:s.i+len(w)]) != w {
		return slow
	}
	s.i += len(w)
	return nil
}

func (s *scanner) array(item func() error) error {
	if s.next() != '[' {
		return slow
	}
	s.i++
	if s.next() == ']' {
		s.i++
		return nil
	}
	for {
		if err := item(); err != nil {
			return err
		}
		switch s.next() {
		case ',':
			s.i++
		case ']':
			s.i++
			return nil
		default:
			return slow
		}
	}
}

// number reads a plain integer: digits, maybe a minus, no leading zero.
func (s *scanner) number() (int64, error) {
	s.space()
	start := s.i
	if s.i < len(s.b) && s.b[s.i] == '-' {
		s.i++
	}
	digits := s.i
	for s.i < len(s.b) && s.b[s.i] >= '0' && s.b[s.i] <= '9' {
		s.i++
	}
	if s.i == digits || s.b[digits] == '0' && s.i-digits > 1 {
		return 0, slow
	}
	if s.i < len(s.b) && (s.b[s.i] == '.' || s.b[s.i] == 'e' || s.b[s.i] == 'E') {
		return 0, slow
	}
	n, err := strconv.ParseInt(string(s.b[start:s.i]), 10, 64)
	if err != nil {
		return 0, slow
	}
	return n, nil
}

// end checks nothing follows.
func (s *scanner) end() error {
	if s.space(); s.i != len(s.b) {
		return slow
	}
	return nil
}

// equalFold is ASCII case folding: keys here are ASCII.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// decodeFast is Decode's one pass, or slow.
func decodeFast(data []byte) (Pack, error) {
	var p Pack
	s := &scanner{b: data}
	err := s.object(func(k string) error {
		switch {
		case equalFold(k, "clock"):
			if s.next() == '-' { // even -0 isn't a uint64 to encoding/json
				return slow
			}
			n, err := s.number()
			if err != nil {
				return slow
			}
			p.Clock = uint64(n)
			return nil
		case equalFold(k, "nonce"):
			var err error
			p.Nonce, err = s.str()
			return err
		case equalFold(k, "ops"):
			if s.next() != '[' {
				return slow
			}
			p.Ops = []Op{}
			return s.array(func() error {
				op, err := s.op()
				p.Ops = append(p.Ops, op)
				return err
			})
		}
		return s.value()
	})
	if err != nil {
		return p, err
	}
	return p, s.end()
}

// op reads one operation, its data as its bytes, checked.
func (s *scanner) op() (Op, error) {
	var op Op
	err := s.object(func(k string) error {
		var err error
		switch {
		case equalFold(k, "kind"):
			op.Kind, err = s.str()
		case equalFold(k, "author"):
			op.Author, err = s.str()
		case equalFold(k, "time"):
			op.Time, err = s.number()
		case equalFold(k, "origin"):
			if s.next() != '{' {
				return slow
			}
			op.Origin = &Origin{}
			err = s.object(func(k string) error {
				var err error
				switch {
				case equalFold(k, "forge"):
					op.Origin.Forge, err = s.str()
				case equalFold(k, "event"):
					op.Origin.Event, err = s.str()
				default:
					err = s.value()
				}
				return err
			})
		case equalFold(k, "data"):
			start := s.next()
			if start == 'n' { // null: no data, as encoding/json leaves it
				return slow
			}
			from := s.i
			if err = s.value(); err == nil {
				op.Data = json.RawMessage(s.b[from:s.i:s.i])
			}
		default:
			err = s.value()
		}
		return err
	})
	return op, err
}

// Fields reads a JSON object of an operation's data in one pass,
// calling field with each key; field reads the value with the Reader. It
// is false when the object isn't one this pass vouches for: then decode
// it with encoding/json, to read it as v1 does.
func Fields(data []byte, field func(key string, r *Reader) error) bool {
	if len(data) == 0 {
		return true
	}
	s := &scanner{b: data}
	r := &Reader{s}
	err := s.object(func(k string) error { return field(k, r) })
	return err == nil && s.end() == nil
}

// Reader reads a value of an operation's data, for Fields.
type Reader struct{ s *scanner }

// String reads a string; null leaves it as it was, as encoding/json does.
func (r *Reader) String(v *string) error {
	if r.s.next() == 'n' {
		return r.s.lit("null")
	}
	x, err := r.s.str()
	*v = x
	return err
}

// Strings reads an array of strings.
func (r *Reader) Strings(v *[]string) error {
	if r.s.next() == 'n' {
		*v = nil
		return r.s.lit("null")
	}
	out := []string{}
	err := r.s.array(func() error {
		x, err := r.s.str()
		out = append(out, x)
		return err
	})
	*v = out
	return err
}

// StringMap reads an object of strings.
func (r *Reader) StringMap(v *map[string]string) error {
	if r.s.next() == 'n' {
		*v = nil
		return r.s.lit("null")
	}
	if r.s.next() != '{' {
		return slow
	}
	out := map[string]string{}
	err := r.s.object(func(k string) error {
		x, err := r.s.str()
		out[k] = x
		return err
	})
	*v = out
	return err
}

// Skip reads past a value no field takes.
func (r *Reader) Skip() error { return r.s.value() }
