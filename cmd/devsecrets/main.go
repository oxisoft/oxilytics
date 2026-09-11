// Dev helper: writes a throwaway App Store .p8 and a fake Google service
// account JSON so the server leaves setup mode locally. Never for production.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", "/tmp/oxi-dev/secrets", "output dir")
	flag.Parse()
	_ = os.MkdirAll(*dir, 0o700)
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(ek)
	_ = os.WriteFile(filepath.Join(*dir, "AuthKey.p8"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	rder, _ := x509.MarshalPKCS8PrivateKey(rk)
	sa := map[string]string{"type": "service_account", "client_email": "dev@example.iam.gserviceaccount.com", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rder})), "project_id": "dev"}
	b, _ := json.Marshal(sa)
	_ = os.WriteFile(filepath.Join(*dir, "gplay-sa.json"), b, 0o600)
}
