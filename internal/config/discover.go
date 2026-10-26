package config

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// scpURL is git's scp-like form, [user@]host:path, with no / before the
// colon.
var scpURL = regexp.MustCompile(`^(?:[^@/:]+@)?([^@/:]+):(.+)$`)

// FromRemote is the forge a git remote's URL names: git@host:path,
// ssh://[user@]host[:port]/path or https://host/path, each with or without
// .git. skip says why a URL with a host names none; both are empty for a
// local path.
//
// The web host is the URL's, less a first label ssh, or a first label's
// -ssh: gitlab-ssh.epfl.ch is gitlab.epfl.ch. github.com is GitHub;
// gitlab.com, or a host whose first label holds gitlab, is GitLab.
func FromRemote(raw string) (f Forge, skip string) {
	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return Forge{}, ""
		}
		if u.Scheme != "ssh" && u.Scheme != "https" {
			return Forge{}, "not git@host:path, ssh:// or https://"
		}
		host, path = u.Hostname(), u.Path
	} else if m := scpURL.FindStringSubmatch(raw); m != nil {
		host, path = m[1], m[2]
	} else {
		return Forge{}, ""
	}
	host = strings.ToLower(host)
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if first, rest, ok := strings.Cut(host, "."); ok {
		if first == "ssh" && strings.Contains(rest, ".") {
			host = rest
		} else if s, ok := strings.CutSuffix(first, "-ssh"); ok && s != "" {
			host = s + "." + rest
		}
	}
	first, _, _ := strings.Cut(host, ".")
	switch {
	case !forgeHost.MatchString(host):
		return Forge{}, "not a host name"
	case host == "github.com":
		if !ghProject.MatchString(path) {
			return Forge{}, "not owner/repo on github.com"
		}
		f = Forge{Kind: "github", Host: host, Project: path}
	case host == "gitlab.com" || strings.Contains(first, "gitlab"):
		if !glPath.MatchString(path) {
			return Forge{}, "not a GitLab project's path"
		}
		f = Forge{Kind: "gitlab", Host: host, Project: path, Path: path}
	default:
		return Forge{}, "not github.com, nor a GitLab host"
	}
	f.Sync, f.Discovered = true, true
	return f, ""
}

// Discover are the forges of a repository's remote URLs, each project
// once: named by their kind, then github-2, github-3… as a kind repeats.
// skipped are the URLs with a host that name none, and why.
func Discover(urls []string) (forges []Forge, skipped []string) {
	seen, kinds := map[string]bool{}, map[string]int{}
	for _, u := range urls {
		f, skip := FromRemote(u)
		if skip != "" {
			skipped = append(skipped, u+": "+skip)
		}
		key := strings.ToLower(f.Kind + " " + f.Host + "/" + f.Project)
		if f.Kind == "" || seen[key] {
			continue
		}
		seen[key] = true
		kinds[f.Kind]++
		f.Name = f.Kind
		if n := kinds[f.Kind]; n > 1 {
			f.Name += "-" + strconv.Itoa(n)
		}
		forges = append(forges, f)
	}
	return forges, skipped
}
