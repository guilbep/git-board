// Package avatar says where a person's picture is (docs/prd/avatars.md):
// their github.com account's, by GitHub's numeric user id; else, only when
// the person viewing chose it, Gravatar's, by a hash of the email; else
// none, and the page draws initials. Gravatar is opt-in: it hands a hash
// of every listed person's email to a third party, and a hash of a
// guessable address is reversed by guessing.
package avatar

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/core/people"
)

// Mode is git config board.avatars: where pictures may come from.
type Mode string

const (
	Forge    Mode = "forge"    // github.com accounts (the default)
	Gravatar Mode = "gravatar" // github.com accounts, then Gravatar
	None     Mode = "none"     // initials only: the browser asks nobody
)

// Size is the picture asked for, in pixels: twice the header's 44px
// button, for screens at 2x; one size, so each picture is cached once.
const Size = 88

// ParseMode reads a board.avatars value: unset is Forge; a value that is
// none of the three is Forge too, and comes back as ignored.
func ParseMode(s string) (m Mode, ignored string) {
	switch v := Mode(strings.ToLower(strings.TrimSpace(s))); v {
	case Forge, Gravatar, None:
		return v, ""
	case "":
		return Forge, ""
	default:
		return Forge, s
	}
}

// URL is p's picture under m, "" for none. The first github.com account
// (Accounts are sorted by server, then id) wins over the email; another
// forge's account gives none, its pictures need an API call (later: kept
// at pull).
func URL(p people.Person, m Mode) string {
	if m == None {
		return ""
	}
	for _, a := range p.Accounts {
		if a.Server != "github.com" {
			continue
		}
		if _, err := strconv.ParseUint(a.ID, 10, 64); err != nil {
			continue // not GitHub's numeric id: never a login in the URL
		}
		return "https://avatars.githubusercontent.com/u/" + a.ID + "?s=" + strconv.Itoa(Size)
	}
	email := strings.ToLower(strings.TrimSpace(p.Email))
	if m != Gravatar || email == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(email))
	q := url.Values{"s": {strconv.Itoa(Size)}, "d": {"404"}}
	return "https://gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?" + q.Encode()
}
