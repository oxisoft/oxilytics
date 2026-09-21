// Package tester implements the live "Test connection" for each store.
package tester

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/setup"
	"github.com/oxisoft/oxilytics/internal/storeclient"
	"github.com/oxisoft/oxilytics/internal/storeclient/appstoreconnect"
	"github.com/oxisoft/oxilytics/internal/storeclient/googleplay"
)

// lookupProbeLimit caps how many apps the connection test probes against the
// public storefront: enough to tell "nothing is released yet" from "the
// endpoint is unreachable", without a slow serial walk of a large account.
const lookupProbeLimit = 5

type Tester struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Tester { return &Tester{cfg: cfg} }

func (t *Tester) Test(ctx context.Context, st models.Store) setup.TestResult {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	res := setup.TestResult{OK: true}
	switch st {
	case models.StoreAppStore:
		t.testASC(ctx, &res)
	case models.StoreGooglePlay:
		t.testGPlay(ctx, &res)
	default:
		res.Step("store", false, "unknown store")
	}
	return res
}

func explain(err error) string {
	switch {
	case errors.Is(err, storeclient.ErrAuth):
		return "401 — credentials rejected: " + err.Error()
	case errors.Is(err, storeclient.ErrForbidden):
		return "403 — permission missing: " + err.Error()
	case errors.Is(err, storeclient.ErrNotFound):
		return "404 — " + err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	}
	return err.Error()
}

// NewASC builds an App Store Connect client from config.
func NewASC(cfg *config.Config) (*appstoreconnect.Client, error) {
	p8, err := os.ReadFile(cfg.ASC.KeyFile)
	if err != nil {
		return nil, err
	}
	return appstoreconnect.New(cfg.ASC.KeyID, cfg.ASC.IssuerID, p8)
}

// NewGPlay builds a Google Play client from config.
func NewGPlay(cfg *config.Config) (*googleplay.Client, error) {
	sa, err := os.ReadFile(cfg.GPlay.SAFile)
	if err != nil {
		return nil, err
	}
	return googleplay.New(sa, cfg.GPlay.Bucket)
}

func (t *Tester) testASC(ctx context.Context, res *setup.TestResult) {
	c, err := NewASC(t.cfg)
	if err != nil {
		res.Step("load key", false, err.Error())
		return
	}
	res.Step("load key", true, "")

	apps, err := c.Apps(ctx)
	if err != nil {
		res.Step("authenticate (GET /v1/apps)", false, explain(err))
		return
	}
	res.Step("authenticate (GET /v1/apps)", true, fmt.Sprintf("%d apps visible", len(apps)))
	if len(apps) == 0 {
		res.Step("apps", false, "no apps on this team")
		return
	}
	first := apps[0]
	if _, err := c.ReportRequests(ctx, first.ID); err != nil {
		res.Step("analytics reports permission", false, explain(err)+" — the key needs the App Manager or Admin role")
	} else {
		res.Step("analytics reports permission", true, "")
	}
	n := 0
	err = c.Reviews(ctx, first.ID, time.Time{}, func(rs []appstoreconnect.Review) bool { n += len(rs); return false })
	if err != nil {
		res.Step("customer reviews", false, explain(err))
	} else {
		res.Step("customer reviews", true, fmt.Sprintf("readable for %q", first.Name))
	}
	// The public storefront only knows publicly released apps, so a lookup on an
	// arbitrary app says nothing about the credentials — it says whether that one
	// app has shipped. Report it as an observation, and only fail on a genuine
	// transport error (DNS, TLS, proxy), which would break icon and rating sync.
	released, checked := 0, 0
	var transportErr error
	for i := range apps {
		if checked == lookupProbeLimit {
			break
		}
		checked++
		switch _, err := c.Lookup(ctx, apps[i].ID, "us"); {
		case err == nil:
			released++
		case errors.Is(err, storeclient.ErrNotFound):
			// not on the public store yet: expected for a new or unreleased app
		default:
			transportErr = err
		}
	}
	switch {
	case transportErr != nil:
		res.Step("iTunes lookup (public)", false, explain(transportErr))
	case released > 0:
		res.Step("iTunes lookup (public)", true, fmt.Sprintf("%d of %d apps checked are on the public store", released, checked))
	default:
		res.Note("iTunes lookup (public)", fmt.Sprintf("reachable; none of the %d apps checked are publicly released yet, so ratings and icons stay empty until one ships", checked))
	}
}

func (t *Tester) testGPlay(ctx context.Context, res *setup.TestResult) {
	c, err := NewGPlay(t.cfg)
	if err != nil {
		res.Step("load service account", false, err.Error())
		return
	}
	res.Step("load service account", true, c.Email)

	objs, err := c.ListObjects(ctx, "stats/installs/")
	if err != nil {
		res.Step("list reports bucket", false, explain(err)+" — check the bucket name and that the service account was invited with 'download bulk reports'")
		return
	}
	res.Step("list reports bucket", true, fmt.Sprintf("%d install report files", len(objs)))
	if len(objs) == 0 {
		res.Step("report files", false, "bucket is empty — Google generates reports the day after the first installs")
		return
	}
	// Test reviews against a package we actually own. The bucket can contain
	// reports for apps transferred in from another developer account: the
	// service account can read their bulk reports but has no Play Console
	// permission on them, so reviews.list returns 403 for those packages while
	// working perfectly for ours. Testing whichever package sorts first paints
	// the whole card red on a correctly configured install.
	pkgs := make([]string, 0, 8)
	for _, o := range objs {
		if _, p, _, ok := googleplay.ClassifyObject(o.Name); ok && p != "" {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		res.Step("report files", false, "no recognisable installs_<package>_YYYYMM files")
		return
	}
	var lastErr error
	var okPkg string
	tried := 0
	seen := map[string]bool{}
	for _, p := range pkgs {
		if seen[p] {
			continue
		}
		seen[p] = true
		tried++
		if _, err := c.Reviews(ctx, p); err != nil {
			lastErr = err
			continue
		}
		okPkg = p
		break
	}
	switch {
	case okPkg != "":
		res.Step("Android Publisher reviews.list", true, "readable for "+okPkg)
	default:
		res.Step("Android Publisher reviews.list", false,
			fmt.Sprintf("%s — tried %d package(s); enable the API in the Cloud project and grant 'View app information'",
				explain(lastErr), tried))
	}
}
