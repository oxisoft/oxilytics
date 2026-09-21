package appstoreconnect

import "testing"

// Real Apple output. Captured from the live analyticsReports API for
// Neon Grid Sudoku (6744125178) on 2026-09-21, header and all.
//
// These fixtures exist because the previous tests used invented column shapes
// and passed for months while production collected nothing: our report names
// did not exist at Apple, and the install report's real columns were parsed
// wrongly. A test that does not use Apple's actual bytes cannot catch that.

const realDownloadsTSV = "Date\tApp Name\tApp Apple Identifier\tDownload Type\tApp Version\tDevice\tPlatform Version\tSource Type\tPage Type\tPre-Order\tTerritory\tCounts\n" +
	"2026-09-20\tNeon Grid Sudoku\t6744125178\tFirst-time download\t1.3.2\tiPhone\tiOS 18.6\tApp Store search\tNo page\t\tUS\t1\n" +
	"2026-09-20\tNeon Grid Sudoku\t6744125178\tAuto-update\t1.3.2\tiPhone\tiOS 16.7\tApp referrer\tStore sheet\t\tSE\t1\n" +
	"2026-09-20\tNeon Grid Sudoku\t6744125178\tRedownload\t1.3.2\tiPhone\tiOS 18.6\tApp Store search\tNo page\t\tBR\t3\n"

// Note the trap: this report has BOTH an "Event" column and a "Download Type"
// column. Reading Download Type first files every install under "updates".
const realInstallsTSV = "Date\tApp Name\tApp Apple Identifier\tEvent\tDownload Type\tApp Version\tDevice\tPlatform Version\tSource Type\tPage Type\tApp Download Date\tTerritory\tCounts\tUnique Devices\n" +
	"2026-09-07\tNeon Grid Sudoku\t6744125178\tInstall\tManual update\t1.3.2\tiPhone\tiOS 26.6\tApp Store search\tNo page\t\tUS\t1\t1\n" +
	"2026-09-07\tNeon Grid Sudoku\t6744125178\tInstall\tManual update\t1.3.2\tiPhone\tiOS 26.6\tApp Store search\tProduct page\t\tBR\t2\t2\n" +
	"2026-09-07\tNeon Grid Sudoku\t6744125178\tDelete\tManual update\t1.3.2\tiPhone\tiOS 26.6\tApp Store search\tNo page\t\tUS\t4\t4\n"

func TestParseRealDownloadsReport(t *testing.T) {
	rows, err := ParseReport([]byte(realDownloadsTSV))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}

	var downloads, redownloads, updates int64
	for _, r := range rows {
		downloads += r.Downloads
		redownloads += r.Redownloads
		updates += r.Updates
		if r.Date != "2026-09-20" {
			t.Errorf("date not parsed: %q", r.Date)
		}
	}
	// "Auto-update" must land in updates, not downloads: counting an update
	// as a download would inflate every install figure we report.
	if downloads != 1 {
		t.Errorf("first-time downloads: want 1, got %d", downloads)
	}
	if redownloads != 3 {
		t.Errorf("redownloads: want 3, got %d", redownloads)
	}
	if updates != 1 {
		t.Errorf("auto-update should count as an update, got updates=%d", updates)
	}
	if got := Territory(rows[0].Territory); got != "US" {
		t.Errorf("territory: want US, got %q", got)
	}
}

func TestParseRealInstallsReportPrefersEventColumn(t *testing.T) {
	rows, err := ParseReport([]byte(realInstallsTSV))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}

	var installs, deletions, updates int64
	for _, r := range rows {
		installs += r.Installs
		deletions += r.Deletions
		updates += r.Updates
	}
	if installs != 3 {
		t.Errorf("installs: want 3, got %d", installs)
	}
	if deletions != 4 {
		t.Errorf("deletions: want 4, got %d", deletions)
	}
	// The regression: "Manual update" in Download Type must NOT win over
	// Event=Install/Delete.
	if updates != 0 {
		t.Errorf("Download Type must not override Event; got updates=%d", updates)
	}
}

// The names are the whole bug. If someone "tidies" them back to the names shown
// in the App Store Connect web UI, every sync silently stores zero rows again.
func TestReportNamesAreApplesExactNames(t *testing.T) {
	cases := map[string]string{
		ReportDownloads: "App Downloads Standard",
		ReportInstalls:  "App Store Installation and Deletion Standard",
		ReportCrashes:   "App Crashes",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("report name %q must be exactly %q (verified against the live API)", got, want)
		}
	}
	if len(AllReportNames) != 3 {
		t.Errorf("AllReportNames should list every report we request, got %d", len(AllReportNames))
	}
}
