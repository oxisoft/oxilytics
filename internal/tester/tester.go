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
	if _, err := c.Lookup(ctx, first.ID, "us"); err != nil {
		res.Step("iTunes lookup (public)", false, explain(err))
	} else {
		res.Step("iTunes lookup (public)", true, "")
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
	pkg := ""
	for _, o := range objs {
		if _, p, _, ok := googleplay.ClassifyObject(o.Name); ok {
			pkg = p
			break
		}
	}
	if pkg == "" {
		res.Step("report files", false, "no recognisable installs_<package>_YYYYMM files")
		return
	}
	if _, err := c.Reviews(ctx, pkg); err != nil {
		res.Step("Android Publisher reviews.list", false, explain(err)+" — enable the API in the Cloud project and grant 'View app information'")
	} else {
		res.Step("Android Publisher reviews.list", true, "readable for "+pkg)
	}
}
