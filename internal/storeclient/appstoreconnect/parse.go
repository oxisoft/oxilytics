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
const (
	ReportDownloads = "App Store Downloads"
	ReportInstalls  = "App Store Installation and Deletion"
	ReportCrashes   = "App Crashes"
)

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
		// App Store Downloads: "Download Type" ∈ First-time download / Redownload / Update ... with Counts
		if dt := strings.ToLower(get(rec, "download type")); dt != "" {
			n := num(rec, "counts", "count", "downloads")
			switch {
			case strings.HasPrefix(dt, "first"):
				row.Downloads = n
			case strings.HasPrefix(dt, "redownload"):
				row.Redownloads = n
			case strings.HasPrefix(dt, "update"):
				row.Updates = n
			default:
				row.Downloads = n
			}
		} else {
			// Column-per-metric shapes (installs/deletions, crashes)
			row.Downloads = num(rec, "first-time downloads", "first time downloads")
			row.Redownloads = num(rec, "redownloads")
			row.Updates = num(rec, "updates")
			row.Installs = num(rec, "installations", "installs")
			row.Deletions = num(rec, "deletions", "uninstalls")
			row.Crashes = num(rec, "crashes", "crash count")
			if et := strings.ToLower(get(rec, "event")); et != "" {
				// Installation and Deletion report: Event ∈ Install / Delete with Counts
				n := num(rec, "counts", "count")
				if strings.HasPrefix(et, "delet") {
					row.Deletions = n
				} else {
					row.Installs = n
				}
			}
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
