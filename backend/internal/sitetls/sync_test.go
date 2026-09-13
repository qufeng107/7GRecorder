package sitetls

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTencentSynchronizerStagesValidatedCertificate(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	archive := testCertificateArchive(t, []string{"7g.chat", "www.7g.chat"}, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	actions := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("X-TC-Action")
		actions = append(actions, action)
		if r.Method != http.MethodPost || r.Host != apiHost {
			t.Fatalf("unexpected request method=%s host=%s", r.Method, r.Host)
		}
		authorization := r.Header.Get("Authorization")
		if !strings.Contains(authorization, "Credential=test-secret-id/") || strings.Contains(authorization, "test-secret-key") {
			t.Fatalf("unexpected authorization header %q", authorization)
		}
		w.Header().Set("Content-Type", "application/json")
		switch action {
		case "DescribeCertificates":
			_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"Certificates": []certificateSummary{{CertificateID: "akxHVwKv", Domain: "7g.chat", CertSANs: []string{"7g.chat", "www.7g.chat"}, Status: 1, CertificateType: "SVR", CertEndTime: "2026-12-12 04:59:59"}}}})
		case "DownloadCertificate":
			_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"Content": base64.StdEncoding.EncodeToString(archive), "ContentType": "application/zip"}})
		default:
			t.Fatalf("unexpected Tencent action %q", action)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	synchronizer := &TencentSynchronizer{dataRoot: root, client: server.Client(), endpoint: server.URL, now: func() time.Time { return now }}
	result, err := synchronizer.Sync(context.Background(), SyncRequest{SecretID: "test-secret-id", SecretKey: "test-secret-key", PrimaryDomain: "7g.chat", AdditionalDomains: []string{"www.7g.chat"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.CertificateID != "akxHVwKv" || result.AlreadyCurrent {
		t.Fatalf("unexpected result %+v", result)
	}
	if strings.Join(actions, ",") != "DescribeCertificates,DownloadCertificate" {
		t.Fatalf("unexpected actions %v", actions)
	}
	pending := filepath.Join(root, "tls", "7g.chat", "pending")
	for _, name := range []string{"fullchain.pem", "privkey.pem"} {
		content, readErr := os.ReadFile(filepath.Join(pending, name))
		if readErr != nil || len(content) == 0 {
			t.Fatalf("read staged %s: %v", name, readErr)
		}
	}
	marker, err := os.ReadFile(filepath.Join(pending, "certificate-id"))
	if err != nil || string(marker) != "akxHVwKv\n" {
		t.Fatalf("unexpected certificate marker %q: %v", marker, err)
	}
}

func TestCertificateMaterialValidatesKeyDomainsAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	archive := testCertificateArchive(t, []string{"7g.chat", "www.7g.chat"}, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	chain, key, leaf, err := certificateMaterial(archive, []string{"7g.chat", "www.7g.chat"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) == 0 || len(key) == 0 || !leaf.NotAfter.After(now) {
		t.Fatal("valid material was not returned")
	}
	if _, _, _, err := certificateMaterial(archive, []string{"missing.7g.chat"}, now); err == nil {
		t.Fatal("expected missing SAN to fail")
	}
	if _, _, _, err := certificateMaterial(archive, []string{"7g.chat"}, now.Add(100*24*time.Hour)); err == nil {
		t.Fatal("expected expired certificate to fail")
	}
}

func TestCertificateMaterialRejectsUnsafeArchivePath(t *testing.T) {
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	entry, err := writer.Create("../privkey.pem")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----\n"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := certificateMaterial(output.Bytes(), []string{"7g.chat"}, time.Now()); err == nil {
		t.Fatal("expected unsafe path to fail")
	}
}

func TestSelectCertificateUsesLatestExactDomainMatch(t *testing.T) {
	items := []certificateSummary{
		{CertificateID: "older", Domain: "7g.chat", CertSANs: []string{"7g.chat", "www.7g.chat"}, Status: 1, CertificateType: "SVR", CertEndTime: "2026-10-01 00:00:00"},
		{CertificateID: "wrong", Domain: "other.test", CertSANs: []string{"other.test"}, Status: 1, CertificateType: "SVR", CertEndTime: "2028-10-01 00:00:00"},
		{CertificateID: "newer", Domain: "7g.chat", CertSANs: []string{"7g.chat", "www.7g.chat"}, Status: 1, CertificateType: "SVR", CertEndTime: "2027-10-01 00:00:00"},
	}
	selected, _, err := selectCertificate(items, []string{"7g.chat", "www.7g.chat"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.CertificateID != "newer" {
		t.Fatalf("selected %q", selected.CertificateID)
	}
}

func testCertificateArchive(t *testing.T, domains []string, notBefore, notAfter time.Time) []byte {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domains[0]}, DNSNames: domains, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	certEntry, _ := writer.Create("7g.chat_nginx/fullchain.crt")
	_, _ = certEntry.Write(certificatePEM)
	keyEntry, _ := writer.Create("7g.chat_nginx/privkey.key")
	_, _ = keyEntry.Write(keyPEM)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
