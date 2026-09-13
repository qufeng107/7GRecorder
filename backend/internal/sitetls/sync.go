package sitetls

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultEndpoint = "https://ssl.tencentcloudapi.com"
	apiHost         = "ssl.tencentcloudapi.com"
	apiVersion      = "2019-12-05"
	maxArchiveBytes = 10 << 20
)

type SyncResult struct {
	CertificateID  string
	NotAfter       time.Time
	AlreadyCurrent bool
}

type Synchronizer interface {
	Sync(context.Context, SyncRequest) (SyncResult, error)
}

type TencentSynchronizer struct {
	dataRoot string
	client   *http.Client
	endpoint string
	now      func() time.Time
}

func NewTencentSynchronizer(dataRoot string) *TencentSynchronizer {
	return &TencentSynchronizer{dataRoot: dataRoot, client: &http.Client{Timeout: 30 * time.Second}, endpoint: defaultEndpoint, now: time.Now}
}

type certificateSummary struct {
	CertificateID   string   `json:"CertificateId"`
	Domain          string   `json:"Domain"`
	CertSANs        []string `json:"CertSANs"`
	SubjectAltName  []string `json:"SubjectAltName"`
	Status          int      `json:"Status"`
	CertificateType string   `json:"CertificateType"`
	CertEndTime     string   `json:"CertEndTime"`
}

func (s *TencentSynchronizer) Sync(ctx context.Context, request SyncRequest) (SyncResult, error) {
	certificates, err := s.listCertificates(ctx, request)
	if err != nil {
		return SyncResult{}, err
	}
	selected, notAfter, err := selectCertificate(certificates, append([]string{request.PrimaryDomain}, request.AdditionalDomains...))
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{CertificateID: selected.CertificateID, NotAfter: notAfter}
	if selected.CertificateID == request.DeployedCertificateID {
		result.AlreadyCurrent = true
		return result, nil
	}
	archive, err := s.downloadCertificate(ctx, request, selected.CertificateID)
	if err != nil {
		return SyncResult{}, err
	}
	fullchain, privateKey, leaf, err := certificateMaterial(archive, append([]string{request.PrimaryDomain}, request.AdditionalDomains...), s.now())
	if err != nil {
		return SyncResult{}, NewClassifiedError("PERMANENT", "validate downloaded certificate", err)
	}
	result.NotAfter = leaf.NotAfter
	pending := filepath.Join(s.dataRoot, "tls", request.PrimaryDomain, "pending")
	if err := os.MkdirAll(pending, 0o700); err != nil {
		return SyncResult{}, NewClassifiedError("PERMANENT", "create certificate staging directory", err)
	}
	if err := atomicWrite(filepath.Join(pending, "fullchain.pem"), fullchain, 0o644); err != nil {
		return SyncResult{}, err
	}
	if err := atomicWrite(filepath.Join(pending, "privkey.pem"), privateKey, 0o600); err != nil {
		return SyncResult{}, err
	}
	if err := atomicWrite(filepath.Join(pending, "certificate-id"), []byte(selected.CertificateID+"\n"), 0o644); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func (s *TencentSynchronizer) listCertificates(ctx context.Context, request SyncRequest) ([]certificateSummary, error) {
	payload := map[string]any{"Offset": 0, "Limit": 100, "SearchKey": request.PrimaryDomain, "CertificateStatus": []int{1}, "CertificateType": "SVR", "ExpirationSort": "DESC"}
	var response struct {
		Response struct {
			Certificates []certificateSummary `json:"Certificates"`
			Error        *apiError            `json:"Error"`
		} `json:"Response"`
	}
	if err := s.call(ctx, request, "DescribeCertificates", payload, &response); err != nil {
		return nil, err
	}
	if response.Response.Error != nil {
		return nil, classifyAPIError(response.Response.Error)
	}
	return response.Response.Certificates, nil
}

func (s *TencentSynchronizer) downloadCertificate(ctx context.Context, request SyncRequest, certificateID string) ([]byte, error) {
	var response struct {
		Response struct {
			Content     string    `json:"Content"`
			ContentType string    `json:"ContentType"`
			Error       *apiError `json:"Error"`
		} `json:"Response"`
	}
	if err := s.call(ctx, request, "DownloadCertificate", map[string]string{"CertificateId": certificateID}, &response); err != nil {
		return nil, err
	}
	if response.Response.Error != nil {
		return nil, classifyAPIError(response.Response.Error)
	}
	archive, err := base64.StdEncoding.DecodeString(response.Response.Content)
	if err != nil || len(archive) == 0 || len(archive) > maxArchiveBytes {
		return nil, NewClassifiedError("PERMANENT", "Tencent SSL returned an invalid certificate archive", err)
	}
	return archive, nil
}

func (s *TencentSynchronizer) call(ctx context.Context, credential SyncRequest, action string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	timestamp := now.Unix()
	date := now.Format("2006-01-02")
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + apiHost + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\ncontent-type;host;x-tc-action\n" + sha256Hex(body)
	scope := date + "/ssl/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + fmt.Sprint(timestamp) + "\n" + scope + "\n" + sha256Hex([]byte(canonicalRequest))
	secretDate := hmacSHA256([]byte("TC3"+credential.SecretKey), date)
	secretService := hmacSHA256(secretDate, "ssl")
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	authorization := "TC3-HMAC-SHA256 Credential=" + credential.SecretID + "/" + scope + ", SignedHeaders=content-type;host;x-tc-action, Signature=" + signature
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", apiHost)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", fmt.Sprint(timestamp))
	req.Header.Set("X-TC-Version", apiVersion)
	resp, err := s.client.Do(req)
	if err != nil {
		return NewClassifiedError("TRANSIENT", "call Tencent SSL API", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxArchiveBytes+1)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewClassifiedError("TRANSIENT", fmt.Sprintf("Tencent SSL API returned HTTP %d", resp.StatusCode), nil)
	}
	if err := json.NewDecoder(limited).Decode(output); err != nil {
		return NewClassifiedError("TRANSIENT", "decode Tencent SSL API response", err)
	}
	return nil
}

type apiError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func classifyAPIError(value *apiError) error {
	class := "TRANSIENT"
	if strings.Contains(strings.ToLower(value.Code), "auth") || strings.Contains(strings.ToLower(value.Code), "permission") {
		class = "AUTH"
	}
	return NewClassifiedError(class, "Tencent SSL API: "+value.Code+": "+value.Message, nil)
}

func selectCertificate(items []certificateSummary, domains []string) (certificateSummary, time.Time, error) {
	type candidate struct {
		item   certificateSummary
		expiry time.Time
	}
	values := make([]candidate, 0)
	for _, item := range items {
		if item.Status != 1 || !strings.EqualFold(item.CertificateType, "SVR") || item.CertificateID == "" {
			continue
		}
		names := append(append([]string{}, item.CertSANs...), item.SubjectAltName...)
		if item.Domain != "" {
			names = append(names, item.Domain)
		}
		if !metadataCovers(names, domains) {
			continue
		}
		expiry, err := time.ParseInLocation("2006-01-02 15:04:05", item.CertEndTime, time.Local)
		if err != nil {
			continue
		}
		values = append(values, candidate{item: item, expiry: expiry})
	}
	if len(values) == 0 {
		return certificateSummary{}, time.Time{}, NewClassifiedError("PERMANENT", "no issued Tencent SSL certificate covers all configured domains", nil)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].expiry.After(values[j].expiry) })
	return values[0].item, values[0].expiry, nil
}

func metadataCovers(names, domains []string) bool {
	for _, domain := range domains {
		covered := false
		for _, name := range names {
			if strings.EqualFold(strings.TrimSpace(name), domain) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func certificateMaterial(archive []byte, domains []string, now time.Time) ([]byte, []byte, *x509.Certificate, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, nil, nil, err
	}
	var certs, keys [][]byte
	var total int64
	for _, file := range reader.File {
		clean := filepath.Clean(file.Name)
		if file.FileInfo().IsDir() {
			continue
		}
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, nil, nil, errors.New("certificate archive contains an unsafe path")
		}
		if file.UncompressedSize64 > maxArchiveBytes {
			return nil, nil, nil, errors.New("certificate archive entry is too large")
		}
		rc, err := file.Open()
		if err != nil {
			return nil, nil, nil, err
		}
		content, readErr := io.ReadAll(io.LimitReader(rc, maxArchiveBytes+1))
		rc.Close()
		if readErr != nil {
			return nil, nil, nil, readErr
		}
		total += int64(len(content))
		if total > maxArchiveBytes {
			return nil, nil, nil, errors.New("certificate archive is too large")
		}
		if bytes.Contains(content, []byte("-----BEGIN CERTIFICATE-----")) {
			certs = append(certs, content)
		}
		if bytes.Contains(content, []byte("PRIVATE KEY-----")) {
			keys = append(keys, content)
		}
	}
	for _, cert := range certs {
		bundle := append([]byte{}, cert...)
		for _, other := range certs {
			if !bytes.Equal(other, cert) {
				bundle = append(append(bundle, '\n'), other...)
			}
		}
		for _, key := range keys {
			pair, err := tls.X509KeyPair(bundle, key)
			if err != nil || len(pair.Certificate) == 0 {
				continue
			}
			leaf, err := x509.ParseCertificate(pair.Certificate[0])
			if err != nil {
				continue
			}
			if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
				continue
			}
			valid := true
			for _, domain := range domains {
				if leaf.VerifyHostname(domain) != nil {
					valid = false
					break
				}
			}
			if valid {
				return bundle, key, leaf, nil
			}
		}
	}
	return nil, nil, nil, errors.New("archive does not contain a valid matching certificate and private key")
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func sha256Hex(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func hmacSHA256(key []byte, value string) []byte {
	hash := hmac.New(sha256.New, key)
	_, _ = hash.Write([]byte(value))
	return hash.Sum(nil)
}

type ClassifiedError struct {
	class, message string
	cause          error
}

func NewClassifiedError(class, message string, cause error) error {
	return &ClassifiedError{class: class, message: message, cause: cause}
}
func (e *ClassifiedError) Error() string {
	if e.cause != nil {
		return e.message + ": " + e.cause.Error()
	}
	return e.message
}
func (e *ClassifiedError) Unwrap() error      { return e.cause }
func (e *ClassifiedError) ErrorClass() string { return e.class }
