package setup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/models"
)

func writeP8(t *testing.T, dir string) string {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	p := filepath.Join(dir, "AuthKey.p8")
	os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
	return p
}

func TestEvaluate(t *testing.T) {
	dir := t.TempDir()
	p8 := writeP8(t, dir)
	sa := filepath.Join(dir, "sa.json")
	os.WriteFile(sa, []byte(`{"type":"service_account","client_email":"x@p.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n","project_id":"p"}`), 0o600)

	tests := []struct {
		name          string
		cfg           config.Config
		wantASC       bool
		wantGP        bool
		setupRequired bool
	}{
		{"nothing", config.Config{ASC: config.ASCConfig{KeyFile: "/nope"}, GPlay: config.GPlayConfig{SAFile: "/nope"}}, false, false, true},
		{"asc only", config.Config{ASC: config.ASCConfig{KeyID: "K", IssuerID: "I", KeyFile: p8}, GPlay: config.GPlayConfig{SAFile: "/nope"}}, true, false, false},
		{"asc missing issuer", config.Config{ASC: config.ASCConfig{KeyID: "K", KeyFile: p8}, GPlay: config.GPlayConfig{SAFile: "/nope"}}, false, false, true},
		{"asc bad key", config.Config{ASC: config.ASCConfig{KeyID: "K", IssuerID: "I", KeyFile: sa}, GPlay: config.GPlayConfig{SAFile: "/nope"}}, false, false, true},
		{"gplay only", config.Config{ASC: config.ASCConfig{KeyFile: "/nope"}, GPlay: config.GPlayConfig{SAFile: sa, Bucket: "pubsite_prod_rev_1"}}, false, true, false},
		{"gplay no bucket", config.Config{ASC: config.ASCConfig{KeyFile: "/nope"}, GPlay: config.GPlayConfig{SAFile: sa}}, false, false, true},
		{"both", config.Config{ASC: config.ASCConfig{KeyID: "K", IssuerID: "I", KeyFile: p8}, GPlay: config.GPlayConfig{SAFile: sa, Bucket: "b"}}, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := Evaluate(&tt.cfg)
			if st.Configured(models.StoreAppStore) != tt.wantASC {
				t.Errorf("asc configured = %v; checks %+v", !tt.wantASC, st.Stores[models.StoreAppStore].Checks)
			}
			if st.Configured(models.StoreGooglePlay) != tt.wantGP {
				t.Errorf("gplay configured = %v; checks %+v", !tt.wantGP, st.Stores[models.StoreGooglePlay].Checks)
			}
			if st.SetupRequired != tt.setupRequired {
				t.Errorf("setup_required = %v", st.SetupRequired)
			}
		})
	}
}
