// Package release holds the pure version logic for client-side releases:
// tags, the next version, candidate numbering and the dev version.
package release

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

// Version is a SemVer version. RC 0 means a final release.
type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
	RC    int `json:"rc"`
}

var tagPattern = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-rc\.([1-9]\d*))?$`)

// ParseTag reads "v1.4.20" or "v1.4.20-rc.2". Anything else reports false.
func ParseTag(tag string) (Version, bool) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return Version{}, false
	}
	var v Version
	nums := []*int{&v.Major, &v.Minor, &v.Patch, &v.RC}
	for i, s := range m[1:] {
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return Version{}, false
		}
		*nums[i] = n
	}
	return v, true
}

// Tag returns the git tag form, "v1.4.20-rc.2".
func (v Version) Tag() string { return "v" + v.String() }

// String returns the version without the "v", "1.4.20-rc.2".
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.RC > 0 {
		s += fmt.Sprintf("-rc.%d", v.RC)
	}
	return s
}

// Final returns the final release of this version (RC 0).
func (v Version) Final() Version {
	v.RC = 0
	return v
}

// Less reports SemVer precedence: 1.4.20-rc.2 < 1.4.20 < 1.4.21-rc.1.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	if v.Patch != w.Patch {
		return v.Patch < w.Patch
	}
	// A final sorts after any candidate of the same numbers.
	if v.RC == 0 || w.RC == 0 {
		return v.RC != 0 && w.RC == 0
	}
	return v.RC < w.RC
}

// Bump returns the next final version after last. A breaking change bumps
// the major, a "feat" bumps the minor, anything else bumps the patch.
func Bump(last Version, kinds []string, breaking bool) Version {
	next := last.Final()
	switch {
	case breaking:
		return Version{Major: last.Major + 1}
	case slices.Contains(kinds, "feat"):
		return Version{Major: last.Major, Minor: last.Minor + 1}
	default:
		next.Patch++
		return next
	}
}

// NextRC returns the next candidate of final: one more than the highest RC
// already taken for the same Major.Minor.Patch, or 1.
func NextRC(final Version, taken []Version) Version {
	base := final.Final()
	next := Version{Major: base.Major, Minor: base.Minor, Patch: base.Patch, RC: 1}
	for _, t := range taken {
		if t.Major == base.Major && t.Minor == base.Minor && t.Patch == base.Patch && t.RC >= next.RC {
			next.RC = t.RC + 1
		}
	}
	return next
}

// DevVersion names a build past lastFinal: the next patch, with the count of
// commits since and the short sha, "1.4.20-dev.7.g8c41d2e".
func DevVersion(lastFinal Version, since int, sha string) string {
	if len(sha) > 7 {
		sha = sha[:7]
	}
	next := lastFinal.Final()
	next.Patch++
	return fmt.Sprintf("%s-dev.%d.g%s", next.String(), since, sha)
}
