package upload

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type cosDeleteTransport func(*http.Request) (*http.Response, error)

func (f cosDeleteTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCOSDeletePinnedSDK(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		name, body string
		code       int
		wantErr    bool
	}{
		{"success", "", 204, false},
		{"already missing", `<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`, 404, false},
		{"denied", `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`, 403, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			http.DefaultTransport = cosDeleteTransport(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Method != "DELETE" || r.URL.Host != "test-123.cos.ap-shanghai.myqcloud.com" || r.URL.Path != "/archive/test.flv" {
					t.Fatalf("unexpected delete %s %s", r.Method, r.URL)
				}
				return &http.Response{StatusCode: tc.code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
			})
			err := NewTencentCOSUploader().Delete(context.Background(), COSUploadRequest{Region: "ap-shanghai", Bucket: "test-123", Prefix: "archive/", ObjectKey: "archive/test.flv", Secret: COSSecret{SecretID: "fake", SecretKey: "fake"}})
			if (err != nil) != tc.wantErr || !called {
				t.Fatalf("called=%v err=%v", called, err)
			}
		})
	}
}
func TestCOSDeleteRejectsUnsafeKey(t *testing.T) {
	for _, key := range []string{"archive-other/test.flv", "archive/../test.flv", "archive/", "/archive/test.flv", "archive/a\\b"} {
		if err := NewTencentCOSUploader().Delete(context.Background(), COSUploadRequest{Region: "ap-shanghai", Bucket: "test-123", Prefix: "archive/", ObjectKey: key}); err == nil {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
}
