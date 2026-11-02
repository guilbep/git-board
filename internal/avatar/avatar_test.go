package avatar

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/guilbep/git-board/core/people"
)

func TestURL(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("ann@epfl.ch"))
	grav := "https://gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?d=404&s=88"
	gh := func(id string) people.Account { return people.Account{Server: "github.com", ID: id, Login: "ann"} }
	for _, c := range []struct {
		name string
		p    people.Person
		m    Mode
		want string
	}{
		{"none sends nothing", people.Person{Email: "ann@epfl.ch", Accounts: []people.Account{gh("17847282")}}, None, ""},
		{"github", people.Person{Accounts: []people.Account{gh("17847282")}}, Forge, "https://avatars.githubusercontent.com/u/17847282?s=88"},
		{"gravatar is opt-in", people.Person{Email: "ann@epfl.ch"}, Forge, ""},
		{"gravatar", people.Person{Email: "ann@epfl.ch"}, Gravatar, grav},
		{"the email trimmed and lower-cased", people.Person{Email: " Ann@EPFL.ch "}, Gravatar, grav},
		{"the forge first", people.Person{Email: "ann@epfl.ch", Accounts: []people.Account{gh("9")}}, Gravatar, "https://avatars.githubusercontent.com/u/9?s=88"},
		{"github among others", people.Person{Accounts: []people.Account{{Server: "gitlab.com", ID: "5"}, gh("9")}}, Forge, "https://avatars.githubusercontent.com/u/9?s=88"},
		{"the first github account", people.Person{Accounts: []people.Account{gh("12"), gh("34")}}, Forge, "https://avatars.githubusercontent.com/u/12?s=88"},
		{"github enterprise", people.Person{Accounts: []people.Account{{Server: "github.example.org", ID: "3"}}}, Forge, ""},
		{"gitlab", people.Person{Accounts: []people.Account{{Server: "gitlab.com", ID: "5"}}}, Gravatar, ""},
		{"a login for an id", people.Person{Accounts: []people.Account{gh("ann")}}, Forge, ""},
		{"no email", people.Person{}, Gravatar, ""},
	} {
		if got := URL(c.p, c.m); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseMode(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in, ignored string
		want        Mode
	}{
		{"", "", Forge}, {"forge", "", Forge}, {" Gravatar ", "", Gravatar}, {"none", "", None}, {"faces", "faces", Forge},
	} {
		if m, ig := ParseMode(c.in); m != c.want || ig != c.ignored {
			t.Errorf("ParseMode(%q) = %q, %q", c.in, m, ig)
		}
	}
}
