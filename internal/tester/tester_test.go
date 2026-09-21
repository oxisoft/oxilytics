package tester

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oxisoft/oxilytics/internal/setup"
	"github.com/oxisoft/oxilytics/internal/storeclient/googleplay"
)

// A Play reports bucket can hold apps transferred in from another developer
// account. The service account reads their bulk reports fine but has no Play
// Console permission on them, so reviews.list returns 403 for those packages
// and 200 for ours.
//
// The first implementation probed whichever package appeared first in the
// bucket listing. With a foreign package sorting first, a fully working
// install reported "Connection failed" — which is exactly what happened on
// production with by.marketcom.droidapp.
type fakeReviewer struct {
	allowed map[string]bool
	calls   []string
}

func (f *fakeReviewer) reviews(pkg string) error {
	f.calls = append(f.calls, pkg)
	if f.allowed[pkg] {
		return nil
	}
	return errors.New("HTTP 403: PERMISSION_DENIED")
}

// probeReviews mirrors the package-selection logic in testGPlay.
func probeReviews(objs []string, rev func(string) error, res *setup.TestResult) {
	pkgs := make([]string, 0, 8)
	for _, name := range objs {
		if _, p, _, ok := googleplay.ClassifyObject(name); ok && p != "" {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		res.Step("report files", false, "no recognisable installs_<package>_YYYYMM files")
		return
	}
	var lastErr error
	var okPkg string
	seen := map[string]bool{}
	for _, p := range pkgs {
		if seen[p] {
			continue
		}
		seen[p] = true
		if err := rev(p); err != nil {
			lastErr = err
			continue
		}
		okPkg = p
		break
	}
	if okPkg != "" {
		res.Step("Android Publisher reviews.list", true, "readable for "+okPkg)
		return
	}
	res.Step("Android Publisher reviews.list", false, lastErr.Error())
}

func TestReviewsProbeSkipsForeignPackages(t *testing.T) {
	objs := []string{
		// sorts first, not ours — the production trap
		"stats/installs/installs_by.marketcom.droidapp_202509_overview.csv",
		"stats/installs/installs_io.oxisoft.sudoku_202509_overview.csv",
	}
	f := &fakeReviewer{allowed: map[string]bool{"io.oxisoft.sudoku": true}}
	res := &setup.TestResult{OK: true}

	probeReviews(objs, f.reviews, res)

	if !res.OK {
		t.Fatalf("connection reported failed despite an owned package being readable: %+v", res.Steps)
	}
	last := res.Steps[len(res.Steps)-1]
	if !strings.Contains(last.Detail, "io.oxisoft.sudoku") {
		t.Fatalf("expected the owned package in the detail, got %q", last.Detail)
	}
	if len(f.calls) != 2 {
		t.Fatalf("expected the foreign package to be tried then skipped, calls=%v", f.calls)
	}
}

func TestReviewsProbeFailsWhenNothingReadable(t *testing.T) {
	objs := []string{
		"stats/installs/installs_by.marketcom.droidapp_202509_overview.csv",
		"stats/installs/installs_io.oxisoft.sudoku_202509_overview.csv",
	}
	f := &fakeReviewer{allowed: map[string]bool{}}
	res := &setup.TestResult{OK: true}

	probeReviews(objs, f.reviews, res)

	if res.OK {
		t.Fatal("a genuinely broken API must still fail the connection test")
	}
	if len(f.calls) != 2 {
		t.Fatalf("every distinct package should be tried, calls=%v", f.calls)
	}
}

func TestReviewsProbeDeduplicatesPackages(t *testing.T) {
	// One package produces many monthly files; probing each would be slow and
	// would hammer the API on a large account.
	objs := []string{
		"stats/installs/installs_io.oxisoft.sudoku_202507_overview.csv",
		"stats/installs/installs_io.oxisoft.sudoku_202508_overview.csv",
		"stats/installs/installs_io.oxisoft.sudoku_202509_overview.csv",
	}
	f := &fakeReviewer{allowed: map[string]bool{"io.oxisoft.sudoku": true}}
	res := &setup.TestResult{OK: true}

	probeReviews(objs, f.reviews, res)

	if len(f.calls) != 1 {
		t.Fatalf("expected one call for one distinct package, got %v", f.calls)
	}
	_ = context.Background()
}
