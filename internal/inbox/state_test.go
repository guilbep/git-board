package inbox_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/inbox"
)

func TestStateRoundTrip(t *testing.T) {
	fs := gitbugiotest.New(t).Storage()
	st, err := inbox.Load(fs, "me1")
	if err != nil || !reflect.DeepEqual(st, inbox.State{}) {
		t.Fatalf("no file yet: %+v, %v; want an empty State", st, err)
	}
	st.MarkSeen(gitbugio.Thread{Issue: gitbugio.Issue{ID: "a1"}, Events: []gitbugio.Event{{ID: "e1"}, {ID: "e2"}}})
	yes, no := true, false
	st.SetFollow("b2", &yes)
	st.SetFollow("c3", &no)
	st.Views, st.Fields = []string{"backend"}, true
	if err := inbox.Save(fs, "me1", st); err != nil {
		t.Fatal(err)
	}
	got, err := inbox.Load(fs, "me1")
	if err != nil || !reflect.DeepEqual(got, st) {
		t.Fatalf("Load after Save = %+v, %v; want %+v", got, err, st)
	}
	if other, _ := inbox.Load(fs, "me2"); !reflect.DeepEqual(other, inbox.State{}) {
		t.Errorf("another identity's file: %+v, want empty", other)
	}
}

func TestStateUnreadableIsAnError(t *testing.T) {
	fs := gitbugiotest.New(t).Storage()
	if err := fs.MkdirAll("git-board/inbox", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create("git-board/inbox/me1.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("{not json"))
	_ = f.Close()
	if _, err := inbox.Load(fs, "me1"); err == nil || !strings.Contains(err.Error(), "me1.json") {
		t.Errorf("Load of a broken file: %v, want an error naming it", err)
	}
}

func TestFollowAndPrune(t *testing.T) {
	var st inbox.State
	yes, no := true, false
	st.SetFollow("a1", &yes)
	if s := st.Set("a1"); s == nil || !*s {
		t.Errorf("after Follow: %v", s)
	}
	st.SetFollow("a1", &no)
	if s := st.Set("a1"); s == nil || *s || len(st.Follow) != 0 {
		t.Errorf("after Unfollow: %v, follow %v", s, st.Follow)
	}
	st.SetFollow("a1", nil)
	if s := st.Set("a1"); s != nil || len(st.Unfollow) != 0 {
		t.Errorf("back to automatic: %v, unfollow %v", s, st.Unfollow)
	}

	st.Seen = map[string][]string{"old": {"e1"}, "new": {"e2"}}
	st.Prune(map[string]bool{"new": true})
	if !reflect.DeepEqual(st.Seen, map[string][]string{"new": {"e2"}}) {
		t.Errorf("Prune kept %v", st.Seen)
	}
}
