package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The OAuth device flow (RFC 8628) on github.com: git-board asks for a
// code, the person types it at github.com/login/device and approves, and
// git-board gets a token, which goes to git's credential helper (ADR
// 0010). An OAuth App's token does not expire, so no refresh token is
// kept.

// ClientID is git-board's OAuth App on github.com, its device flow on.
// It is public, as gh's is: its owner registers the app and writes the
// id here. A fork sets its own at build time (make GITHUB_CLIENT_ID=…).
// Empty, there is no device flow: a pasted token alone.
var ClientID = ""

// DeviceScopes are what sync asks of a token: repo for a private
// project's issues, labels and releases; project for a GitHub Project's
// fields.
const DeviceScopes = "repo project"

// Device is a device flow started: the code the person types at URL,
// until Expires. Its device code is the flow's secret, and stays here.
type Device struct {
	UserCode string
	URL      string
	Expires  time.Time

	base, clientID, code string
	interval             time.Duration
}

// ErrDenied is a device flow the person turned down; ErrExpired one
// nobody approved in time.
var (
	ErrDenied  = errors.New("github: the login was turned down on GitHub")
	ErrExpired = errors.New("github: the code expired before it was approved")
)

// StartDevice asks base (https://github.com; a loopback fake in tests)
// for a device code for clientID.
func StartDevice(ctx context.Context, base, clientID string) (*Device, error) {
	if clientID == "" {
		return nil, errors.New("github: no OAuth App: the device flow is off in this build")
	}
	base = strings.TrimSuffix(base, "/")
	if err := checkURL(base, "github.com"); err != nil {
		return nil, err
	}
	var res struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		Description     string `json:"error_description"`
	}
	if err := postForm(ctx, base+"/login/device/code", url.Values{"client_id": {clientID}, "scope": {DeviceScopes}}, &res); err != nil {
		return nil, err
	}
	if res.Error != "" || res.DeviceCode == "" {
		return nil, fmt.Errorf("github: no device code: %s %s", res.Error, res.Description)
	}
	return &Device{
		UserCode: res.UserCode, URL: res.VerificationURI,
		Expires: time.Now().Add(time.Duration(res.ExpiresIn) * time.Second),
		base:    base, clientID: clientID, code: res.DeviceCode,
		interval: time.Duration(max(res.Interval, 1)) * time.Second,
	}, nil
}

// Wait polls until the person approves (the token), turns it down
// (ErrDenied), the code expires (ErrExpired), or ctx ends. It waits the
// interval GitHub gives, longer each time GitHub says to slow down.
func (d *Device) Wait(ctx context.Context) (string, error) {
	ctx, cancel := context.WithDeadline(ctx, d.Expires)
	defer cancel()
	for {
		if err := sleep(ctx, d.interval); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return "", ErrExpired
			}
			return "", err
		}
		var res struct {
			Token    string `json:"access_token"`
			Error    string `json:"error"`
			Interval int    `json:"interval"`
		}
		err := postForm(ctx, d.base+"/login/oauth/access_token", url.Values{
			"client_id": {d.clientID}, "device_code": {d.code},
			"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &res)
		switch {
		case err != nil:
			return "", err
		case res.Token != "":
			return res.Token, nil
		case res.Error == "authorization_pending":
		case res.Error == "slow_down":
			d.interval = max(d.interval+5*time.Second, time.Duration(res.Interval)*time.Second)
		case res.Error == "access_denied":
			return "", ErrDenied
		case res.Error == "expired_token":
			return "", ErrExpired
		default:
			return "", fmt.Errorf("github: the login failed: %s", res.Error)
		}
	}
}

// postForm posts form to u and decodes the JSON answer into out. GitHub
// answers the flow's errors as JSON with 200, and some as 4xx: both are
// decoded.
func postForm(ctx context.Context, u string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("github: %s answered %d, not JSON: %w", u, res.StatusCode, err)
	}
	return nil
}
