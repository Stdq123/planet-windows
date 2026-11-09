// Package version defines the release versions shared by the application,
// update publisher and installer. Build metadata is deliberately unsupported:
// one version identifies one immutable set of release bytes.
package version

import (
	"cmp"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var syntax = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type Version struct {
	core [3]uint64
	pre  []string
}

func Parse(text string) (Version, error) {
	var v Version
	match := syntax.FindStringSubmatch(text)
	if match == nil {
		return v, errors.New("invalid release version")
	}
	for i := range v.core {
		n, err := strconv.ParseUint(match[i+1], 10, 32)
		if err != nil {
			return Version{}, errors.New("release version component exceeds uint32")
		}
		v.core[i] = n
	}
	if match[4] != "" {
		v.pre = strings.Split(match[4], ".")
		for _, part := range v.pre {
			if numeric(part) && len(part) > 1 && part[0] == '0' {
				return Version{}, errors.New("numeric prerelease identifier has a leading zero")
			}
		}
	}
	return v, nil
}

func numeric(s string) bool {
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func (v Version) IsPrerelease() bool { return len(v.pre) > 0 }

func (v Version) Compare(other Version) int {
	for i := range v.core {
		if result := cmp.Compare(v.core[i], other.core[i]); result != 0 {
			return result
		}
	}
	if !v.IsPrerelease() || !other.IsPrerelease() {
		return cmp.Compare(len(other.pre), len(v.pre))
	}
	for i := 0; i < min(len(v.pre), len(other.pre)); i++ {
		a, b := v.pre[i], other.pre[i]
		an, bn := numeric(a), numeric(b)
		if an != bn {
			if an {
				return -1
			}
			return 1
		}
		// Canonical numeric identifiers have no leading zeroes, so length then
		// lexical order compares arbitrarily large numbers without overflow.
		if an {
			if result := cmp.Compare(len(a), len(b)); result != 0 {
				return result
			}
		}
		if result := strings.Compare(a, b); result != 0 {
			return result
		}
	}
	return cmp.Compare(len(v.pre), len(other.pre))
}

func Compare(a, b string) (int, error) {
	x, err := Parse(a)
	if err != nil {
		return 0, err
	}
	y, err := Parse(b)
	if err != nil {
		return 0, err
	}
	return x.Compare(y), nil
}
