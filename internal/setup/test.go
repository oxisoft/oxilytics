package setup

import "github.com/oxisoft/oxilytics/internal/models"

// TestResult is what a live connection test returns.
type TestResult struct {
	OK    bool    `json:"ok"`
	Steps []Check `json:"steps"`
}

func (t *TestResult) Step(name string, ok bool, detail string) {
	t.Steps = append(t.Steps, Check{Name: name, OK: ok, Detail: detail})
	if !ok {
		t.OK = false
	}
}

// Note records something the operator should see that is NOT a verdict on the
// credentials — an observation about the account's own data. It never fails the
// test, because nothing the operator could configure would change it.
func (t *TestResult) Note(name, detail string) {
	t.Steps = append(t.Steps, Check{Name: name, OK: true, Info: true, Detail: detail})
}

type EnvVar struct {
	Name    string `json:"name"`
	Example string `json:"example"`
	Help    string `json:"help"`
}

// EnvVars lists the variables a store needs; rendered by the setup guide.
func EnvVars(st models.Store) []EnvVar {
	switch st {
	case models.StoreAppStore:
		return []EnvVar{
			{"OXI_ASC_KEY_ID", "ABC123DEFG", "Key ID shown in App Store Connect → Users and Access → Integrations → Team Keys"},
			{"OXI_ASC_ISSUER_ID", "69a6de7e-xxxx-xxxx-xxxx-xxxxxxxxxxxx", "Issuer ID at the top of the same page"},
			{"OXI_ASC_KEY_FILE", "/secrets/AuthKey_ABC123DEFG.p8", "Path inside the container to the downloaded .p8 key"},
		}
	case models.StoreGooglePlay:
		return []EnvVar{
			{"OXI_GPLAY_SA_FILE", "/secrets/gplay-sa.json", "Path inside the container to the service-account JSON key"},
			{"OXI_GPLAY_BUCKET", "pubsite_prod_rev_01234567890123456789", "Bucket name from Play Console → Download reports → Copy Cloud Storage URI"},
		}
	}
	return nil
}
