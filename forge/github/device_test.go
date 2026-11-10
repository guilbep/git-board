package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A fake github.com for the device flow: the code, then answers in
// order, pending and slow_down first.
func fakeDevice(t *testing.T, answers ...map[string]any) *httptest.Server {
	t.Helper()
	var polls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("client_id") != "cid" || r.FormValue("scope") != DeviceScopes {
			http.Error(w, "bad request", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"device_code": "dc", "user_code": "ABCD-1234", "verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 0})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("device_code") != "dc" || r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			http.Error(w, "bad request", 400)
			return
		}
		i := int(polls.Add(1)) - 1
		json.NewEncoder(w).Encode(answers[min(i, len(answers)-1)])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDeviceFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := fakeDevice(t, map[string]any{"error": "authorization_pending"}, map[string]any{"access_token": "gho_x", "token_type": "bearer"})
	d, err := StartDevice(ctx, srv.URL, "cid")
	if err != nil {
		t.Fatal(err)
	}
	if d.UserCode != "ABCD-1234" || d.URL != "https://github.com/login/device" {
		t.Fatalf("device %+v", d)
	}
	d.interval = 1 // the fake answers at once
	if tok, err := d.Wait(ctx); err != nil || tok != "gho_x" {
		t.Fatalf("token %q, %v", tok, err)
	}

	srv = fakeDevice(t, map[string]any{"error": "access_denied"})
	d, _ = StartDevice(ctx, srv.URL, "cid")
	d.interval = 1
	if _, err := d.Wait(ctx); !errors.Is(err, ErrDenied) {
		t.Fatalf("denied: %v", err)
	}

	srv = fakeDevice(t, map[string]any{"error": "slow_down", "interval": 0}, map[string]any{"error": "expired_token"})
	d, _ = StartDevice(ctx, srv.URL, "cid")
	d.interval = 1
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if _, err := d.Wait(wctx); !errors.Is(err, ErrExpired) || d.interval < 5e9 {
		t.Fatalf("slowed down to %v, then %v", d.interval, err)
	}

	if _, err := StartDevice(ctx, srv.URL, ""); err == nil {
		t.Error("a build with no OAuth App started a flow")
	}
	if _, err := StartDevice(ctx, "https://evil.example.org", "cid"); err == nil {
		t.Error("a flow started on another host")
	}
}
