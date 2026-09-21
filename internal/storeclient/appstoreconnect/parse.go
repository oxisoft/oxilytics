package appstoreconnect

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Report names we consume.
//
// These are Apple's exact report names, verified against the live
// analyticsReports endpoint — not the names shown in the App Store Connect
// web UI, which differ. Apple offers a "Standard" and a "Detailed" variant of
// the download and install reports; we take Standard because it is produced
// DAILY, whereas Detailed is only WEEKLY/MONTHLY. Detailed adds just three
// attribution columns (Source Info, Campaign, Page Title), which are empty for
// apps that run no ad campaigns, so it would cost daily resolution and
// historical depth to gain nothing.
//
// Getting a name wrong is silent: Apple simply does not list that report and
// every sync stores zero rows while reporting success. reportNameUnknown below
// turns that into a hard error for exactly this reason.
const (
	ReportDownloads = "App Downloads Standard"
	ReportInstalls  = "App Store Installation and Deletion Standard"
	ReportCrashes   = "App Crashes"
)

// AllReportNames is every report this ingester asks Apple for. Used to verify
// up front that Apple still offers each one under the expected name.
var AllReportNames = []string{ReportDownloads, ReportInstalls, ReportCrashes}

// Row is one parsed line of an analytics report, normalised to our metrics.
type Row struct {
	Date      string // YYYY-MM-DD
	AppID     string // App Apple Identifier
	Territory string // Apple territory name or ISO code (see Territory())

	Downloads   int64 // first-time downloads
	Redownloads int64
	Updates     int64
	Deletions   int64
	Installs    int64
	Crashes     int64
}

// ParseReport parses a tab-separated analytics report (segment) into rows.
// Unknown columns are ignored; the header decides the mapping so all three
// report types go through the same function.
func ParseReport(data []byte) ([]Row, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	hdr, err := r.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range hdr {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(rec []string, names ...string) string {
		for _, n := range names {
			if i, ok := idx[n]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
		}
		return ""
	}
	num := func(rec []string, names ...string) int64 {
		s := get(rec, names...)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
		if err != nil {
			return 0
		}
		return int64(f + 0.5)
	}

	var rows []Row
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return rows, fmt.Errorf("parse report: %w", err)
		}
		if len(rec) == 0 {
			continue
		}
		row := Row{
			Date:      get(rec, "date"),
			AppID:     get(rec, "app apple identifier", "app id"),
			Territory: get(rec, "territory", "country", "storefront"),
		}
		if row.Date == "" {
			continue
		}
		// Order matters. The Installation and Deletion report carries BOTH an
		// "Event" column (Install/Delete) and a "Download Type" column
		// describing how the app arrived. Checking Download Type first
		// silently filed every install under "updates" and lost deletions
		// entirely, so Event is checked first and wins.
		if et := strings.ToLower(get(rec, "event")); et != "" {
			n := num(rec, "counts", "count")
			if strings.HasPrefix(et, "delet") {
				row.Deletions = n
			} else {
				row.Installs = n
			}
		} else if dt := strings.ToLower(get(rec, "download type")); dt != "" {
			// Downloads report: one row per download type, with Counts.
			n := num(rec, "counts", "count", "downloads")
			switch {
			case strings.HasPrefix(dt, "first"):
				row.Downloads = n
			case strings.HasPrefix(dt, "redownload"):
				row.Redownloads = n
			case strings.HasPrefix(dt, "auto-update"), strings.HasPrefix(dt, "manual update"), strings.HasPrefix(dt, "update"):
				row.Updates = n
			default:
				row.Downloads = n
			}
		} else {
			// Column-per-metric shapes (crashes, and any report that puts each
			// metric in its own column rather than one row per event type).
			row.Downloads = num(rec, "first-time downloads", "first time downloads")
			row.Redownloads = num(rec, "redownloads")
			row.Updates = num(rec, "updates")
			row.Installs = num(rec, "installations", "installs")
			row.Deletions = num(rec, "deletions", "uninstalls")
			row.Crashes = num(rec, "crashes", "crash count")
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Territory normalises Apple territory strings to ISO-3166 alpha-2 where
// known; falls back to "ZZ".
func Territory(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 2 {
		return strings.ToUpper(s)
	}
	if c, ok := territoryNames[strings.ToLower(s)]; ok {
		return c
	}
	return "ZZ"
}

var territoryNames = map[string]string{
	"united states": "US", "united kingdom": "GB", "germany": "DE", "france": "FR", "poland": "PL", "italy": "IT",
	"spain": "ES", "netherlands": "NL", "canada": "CA", "australia": "AU", "japan": "JP", "china mainland": "CN",
	"china": "CN", "india": "IN", "brazil": "BR", "mexico": "MX", "russia": "RU", "sweden": "SE", "norway": "NO",
	"denmark": "DK", "finland": "FI", "switzerland": "CH", "austria": "AT", "belgium": "BE", "ireland": "IE",
	"portugal": "PT", "czechia": "CZ", "czech republic": "CZ", "ukraine": "UA", "turkey": "TR", "türkiye": "TR",
	"korea, republic of": "KR", "south korea": "KR", "taiwan": "TW", "hong kong": "HK", "singapore": "SG",
	"new zealand": "NZ", "south africa": "ZA", "israel": "IL", "united arab emirates": "AE", "saudi arabia": "SA",
	"argentina": "AR", "chile": "CL", "colombia": "CO", "indonesia": "ID", "malaysia": "MY", "philippines": "PH",
	"thailand": "TH", "vietnam": "VN", "hungary": "HU", "romania": "RO", "greece": "GR", "bulgaria": "BG",
	"croatia": "HR", "slovakia": "SK", "slovenia": "SI", "lithuania": "LT", "latvia": "LV", "estonia": "EE",
	"belarus": "BY", "kazakhstan": "KZ", "egypt": "EG", "nigeria": "NG", "kenya": "KE", "pakistan": "PK",
}
