// Package setup decides whether a store is configured and exposes the checks
// the Setup screen renders. Credentials are files/env only.
package setup

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/models"
)

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type StoreStatus struct {
	Store      models.Store      `json:"store"`
	Configured bool              `json:"configured"`
	Checks     []Check           `json:"checks"`
	Info       map[string]string `json:"info"` // non-secret identifiers, for the UI
}

type Status struct {
	SetupRequired bool                         `json:"setup_required"`
	Stores        map[models.Store]StoreStatus `json:"stores"`
}

// Evaluate runs the static (offline) checks for both stores.
func Evaluate(cfg *config.Config) Status {
	st := Status{Stores: map[models.Store]StoreStatus{}}
	asc := checkASC(cfg.ASC)
	gp := checkGPlay(cfg.GPlay)
	st.Stores[models.StoreAppStore] = asc
	st.Stores[models.StoreGooglePlay] = gp
	st.SetupRequired = !asc.Configured && !gp.Configured
	return st
}

func (s Status) Configured(store models.Store) bool { return s.Stores[store].Configured }

func checkASC(c config.ASCConfig) StoreStatus {
	ss := StoreStatus{Store: models.StoreAppStore, Info: map[string]string{
		"key_id": c.KeyID, "issuer_id": c.IssuerID, "key_file": c.KeyFile,
	}}
	add := func(name string, ok bool, detail string) { ss.Checks = append(ss.Checks, Check{name, ok, detail}) }

	add("OXI_ASC_KEY_ID set", c.KeyID != "", "")
	add("OXI_ASC_ISSUER_ID set", c.IssuerID != "", "")
	if c.KeyID == "" && c.IssuerID == "" {
		add("key file", false, "not configured")
		return ss
	}
	b, err := os.ReadFile(c.KeyFile)
	if err != nil {
		add("key file readable", false, err.Error())
		return ss
	}
	add("key file readable", true, "")
	if err := ValidateP8(b); err != nil {
		add("key file is an EC P-256 private key", false, err.Error())
		return ss
	}
	add("key file is an EC P-256 private key", true, "")
	ss.Configured = c.KeyID != "" && c.IssuerID != ""
	return ss
}

// ValidateP8 parses a .p8 (PKCS#8 PEM) and checks it is an ECDSA key.
func ValidateP8(pemBytes []byte) error {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return errors.New("not PEM encoded")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse PKCS#8: %w", err)
	}
	if _, ok := key.(*ecdsa.PrivateKey); !ok {
		return errors.New("not an ECDSA key")
	}
	return nil
}

type serviceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	ProjectID   string `json:"project_id"`
}

func checkGPlay(c config.GPlayConfig) StoreStatus {
	ss := StoreStatus{Store: models.StoreGooglePlay, Info: map[string]string{
		"sa_file": c.SAFile, "bucket": c.Bucket,
	}}
	add := func(name string, ok bool, detail string) { ss.Checks = append(ss.Checks, Check{name, ok, detail}) }

	add("OXI_GPLAY_BUCKET set", c.Bucket != "", "")
	b, err := os.ReadFile(c.SAFile)
	if err != nil {
		if c.Bucket == "" {
			add("service account file", false, "not configured")
		} else {
			add("service account file readable", false, err.Error())
		}
		return ss
	}
	add("service account file readable", true, "")
	var sa serviceAccount
	if err := json.Unmarshal(b, &sa); err != nil {
		add("service account JSON parses", false, err.Error())
		return ss
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		add("service account JSON parses", false, "missing client_email or private_key")
		return ss
	}
	add("service account JSON parses", true, "")
	ss.Info["client_email"] = sa.ClientEmail
	ss.Info["project_id"] = sa.ProjectID
	ss.Configured = c.Bucket != ""
	return ss
}
