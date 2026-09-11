// Package version carries build metadata injected with -ldflags
// (see Makefile / Dockerfile / CI). When not injected, it falls back to the
// VCS information Go embeds automatically in module builds.
package version

import (
	"runtime/debug"
	"strings"
	"time"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
	GoVersion = ""
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	GoVersion = info.GoVersion
	var rev, tm string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			tm = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if Commit == "none" && rev != "" {
		Commit = rev
		if modified {
			Commit += "-dirty"
		}
	}
	if BuildTime == "unknown" && tm != "" {
		if t, err := time.Parse(time.RFC3339, tm); err == nil {
			BuildTime = t.UTC().Format(time.RFC3339)
		}
	}
	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
}

// ShortCommit is the first 7 chars of the commit hash (+ "-dirty" if set).
func ShortCommit() string {
	c := Commit
	dirty := strings.HasSuffix(c, "-dirty")
	c = strings.TrimSuffix(c, "-dirty")
	if len(c) > 7 {
		c = c[:7]
	}
	if dirty {
		c += "-dirty"
	}
	return c
}

// String is the one-line human form: "v1.2.3 (abc1234, built 2026-09-11T08:00:00Z)".
func String() string {
	var b strings.Builder
	b.WriteString(Version)
	if Commit != "none" {
		b.WriteString(" (")
		b.WriteString(ShortCommit())
		if BuildTime != "unknown" {
			b.WriteString(", built ")
			b.WriteString(BuildTime)
		}
		b.WriteString(")")
	}
	return b.String()
}
