package googleplay

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// DecodeUTF16 converts UTF-16 (BOM-prefixed) to UTF-8; passes UTF-8 through.
func DecodeUTF16(b []byte) []byte {
	if len(b) >= 2 {
		le := b[0] == 0xFF && b[1] == 0xFE
		be := b[0] == 0xFE && b[1] == 0xFF
		if le || be {
			b = b[2:]
			u := make([]uint16, 0, len(b)/2)
			for i := 0; i+1 < len(b); i += 2 {
				if le {
					u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
				} else {
					u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
				}
			}
			runes := utf16.Decode(u)
			out := make([]byte, 0, len(runes))
			var tmp [4]byte
			for _, r := range runes {
				n := utf8.EncodeRune(tmp[:], r)
				out = append(out, tmp[:n]...)
			}
			return out
		}
	}
	// strip UTF-8 BOM
	return bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
}

// object name patterns
var (
	reInstalls = regexp.MustCompile(`^stats/installs/installs_(.+)_(\d{6})_(overview|country)\.csv$`)
	reRatings  = regexp.MustCompile(`^stats/ratings/ratings_(.+)_(\d{6})_(overview|country)\.csv$`)
	reCrashes  = regexp.MustCompile(`^stats/crashes/crashes_(.+)_(\d{6})_(overview)\.csv$`)
	reReviews  = regexp.MustCompile(`^reviews/reviews_(.+)_(\d{6})\.csv$`)
)

type ObjectKind string

const (
	KindInstallsOverview ObjectKind = "installs_overview"
	KindInstallsCountry  ObjectKind = "installs_country"
	KindRatingsOverview  ObjectKind = "ratings_overview"
	KindRatingsCountry   ObjectKind = "ratings_country"
	KindCrashesOverview  ObjectKind = "crashes_overview"
	KindReviews          ObjectKind = "reviews"
)

// ClassifyObject returns the kind, package and YYYYMM of a report object.
func ClassifyObject(name string) (kind ObjectKind, pkg, month string, ok bool) {
	if m := reInstalls.FindStringSubmatch(name); m != nil {
		if m[3] == "overview" {
			return KindInstallsOverview, m[1], m[2], true
		}
		return KindInstallsCountry, m[1], m[2], true
	}
	if m := reRatings.FindStringSubmatch(name); m != nil {
		if m[3] == "overview" {
			return KindRatingsOverview, m[1], m[2], true
		}
		return KindRatingsCountry, m[1], m[2], true
	}
	if m := reCrashes.FindStringSubmatch(name); m != nil {
		return KindCrashesOverview, m[1], m[2], true
	}
	if m := reReviews.FindStringSubmatch(name); m != nil {
		return KindReviews, m[1], m[2], true
	}
	return "", "", "", false
}

// StatRow is one line of an installs/ratings/crashes CSV.
type StatRow struct {
	Date    string // YYYY-MM-DD
	Package string
	Country string // "" for overview files

	DailyDeviceInstalls   int64
	DailyDeviceUninstalls int64
	DailyDeviceUpgrades   int64
	DailyUserInstalls     int64
	DailyUserUninstalls   int64
	ActiveDeviceInstalls  *int64
	DailyCrashes          int64
	DailyANRs             int64
	DailyAvgRating        *float64
	TotalAvgRating        *float64
}

func readCSV(data []byte) ([]string, [][]string, error) {
	r := csv.NewReader(bytes.NewReader(DecodeUTF16(data)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	hdr, err := r.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	for i := range hdr {
		hdr[i] = strings.ToLower(strings.TrimSpace(hdr[i]))
	}
	var rows [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return hdr, rows, fmt.Errorf("csv: %w", err)
		}
		rows = append(rows, rec)
	}
	return hdr, rows, nil
}

func col(hdr []string, rec []string, name string) string {
	for i, h := range hdr {
		if h == name && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
	}
	return ""
}

func colInt(hdr []string, rec []string, name string) int64 {
	s := col(hdr, rec, name)
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return int64(n + 0.5)
}

func colIntPtr(hdr []string, rec []string, name string) *int64 {
	s := col(hdr, rec, name)
	if s == "" {
		return nil
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return nil
	}
	v := int64(n + 0.5)
	return &v
}

func colFloatPtr(hdr []string, rec []string, name string) *float64 {
	s := col(hdr, rec, name)
	if s == "" || s == "NA" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// ParseStats parses installs/ratings/crashes CSVs (overview or country).
func ParseStats(data []byte) ([]StatRow, error) {
	hdr, recs, err := readCSV(data)
	if err != nil {
		return nil, err
	}
	var out []StatRow
	for _, rec := range recs {
		date := col(hdr, rec, "date")
		if date == "" {
			continue
		}
		out = append(out, StatRow{
			Date:                  date,
			Package:               col(hdr, rec, "package name"),
			Country:               col(hdr, rec, "country"),
			DailyDeviceInstalls:   colInt(hdr, rec, "daily device installs"),
			DailyDeviceUninstalls: colInt(hdr, rec, "daily device uninstalls"),
			DailyDeviceUpgrades:   colInt(hdr, rec, "daily device upgrades"),
			DailyUserInstalls:     colInt(hdr, rec, "daily user installs"),
			DailyUserUninstalls:   colInt(hdr, rec, "daily user uninstalls"),
			ActiveDeviceInstalls:  colIntPtr(hdr, rec, "active device installs"),
			DailyCrashes:          colInt(hdr, rec, "daily crashes"),
			DailyANRs:             colInt(hdr, rec, "daily anrs"),
			DailyAvgRating:        colFloatPtr(hdr, rec, "daily average rating"),
			TotalAvgRating:        colFloatPtr(hdr, rec, "total average rating"),
		})
	}
	return out, nil
}

// ReviewRow is one line of a reviews_*.csv.
type ReviewRow struct {
	Package        string
	AppVersionCode string
	AppVersionName string
	Language       string
	Device         string
	SubmittedAt    string // raw "2026-09-01T10:11:12Z"
	Rating         int
	Title          string
	Text           string
	ReplyAt        string
	ReplyText      string
	Link           string
	ID             string // derived from Link (reviewId=…)
}

var reReviewID = regexp.MustCompile(`reviewId=([^&]+)`)

func ParseReviews(data []byte) ([]ReviewRow, error) {
	hdr, recs, err := readCSV(data)
	if err != nil {
		return nil, err
	}
	var out []ReviewRow
	for _, rec := range recs {
		link := col(hdr, rec, "review link")
		rr := ReviewRow{
			Package:        col(hdr, rec, "package name"),
			AppVersionCode: col(hdr, rec, "app version code"),
			AppVersionName: col(hdr, rec, "app version name"),
			Language:       col(hdr, rec, "reviewer language"),
			Device:         col(hdr, rec, "device"),
			SubmittedAt:    col(hdr, rec, "review submit date and time"),
			Rating:         int(colInt(hdr, rec, "star rating")),
			Title:          col(hdr, rec, "review title"),
			Text:           col(hdr, rec, "review text"),
			ReplyAt:        col(hdr, rec, "developer reply date and time"),
			ReplyText:      col(hdr, rec, "developer reply text"),
			Link:           link,
		}
		if m := reReviewID.FindStringSubmatch(link); m != nil {
			rr.ID = m[1]
		}
		if rr.ID == "" && rr.SubmittedAt != "" {
			rr.ID = "csv:" + rr.SubmittedAt + ":" + rr.Language + ":" + strconv.Itoa(rr.Rating)
		}
		if rr.SubmittedAt == "" && rr.Rating == 0 {
			continue
		}
		out = append(out, rr)
	}
	return out, nil
}
