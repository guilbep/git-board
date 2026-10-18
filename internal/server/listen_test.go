package server

import (
	"net"
	"slices"
	"testing"
)

func TestListenExposesOnlyWhatAddrNames(t *testing.T) {
	for addr, want := range map[string][]string{"127.0.0.1:0": nil, ":0": {"*"}} {
		ls, exposed, err := listen(addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}
		for _, l := range ls {
			_ = l.Close()
		}
		if !slices.Equal(exposed, want) {
			t.Errorf("%s exposes %v, want %v", addr, exposed, want)
		}
	}
}

// Another server holding 127.0.0.1 (a Docker port) leaves [::1] to localhost.
func TestListenLocalhostSurvivesATakenIPv4(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	_, port, _ := net.SplitHostPort(taken.Addr().String())
	if l, err := net.Listen("tcp", net.JoinHostPort("::1", "0")); err != nil {
		t.Skip("no IPv6 loopback here")
	} else {
		_ = l.Close()
	}
	ls, exposed, err := listen("localhost:" + port)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		for _, l := range ls {
			_ = l.Close()
		}
	}()
	if len(ls) != 1 || exposed != nil {
		t.Errorf("%d listeners, exposed %v; want one on [::1], nothing exposed", len(ls), exposed)
	}
}
