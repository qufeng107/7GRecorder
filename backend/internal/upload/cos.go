package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type COSSecret struct {
	SecretID     string `json:"secret_id"`
	SecretKey    string `json:"secret_key"`
	SessionToken string `json:"session_token"`
}

type COSUploadRequest struct {
	ObjectID           int64
	UploadSourceID     int64
	OutputID           int64
	RecordingProfileID int64
	Region             string
	Bucket             string
	Prefix             string
	ObjectKey          string
	SourcePath         string
	SourceRelativePath string
	SourceSizeBytes    int64
	Secret             COSSecret
}

type COSUploadResult struct {
	ETag      string
	SizeBytes int64
}

type COSDownloadURLRequest struct {
	ObjectID             int64
	UploadSourceID       int64
	UploadSourceOutputID int64
	RecordingProfileID   int64
	Region               string
	Bucket               string
	ObjectKey            string
	Secret               COSSecret
}

type COSDownloadURLResult struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
	ObjectKey string `json:"object_key"`
}

type COSUploader interface {
	Upload(ctx context.Context, request COSUploadRequest, progress ProgressReporter) (COSUploadResult, error)
}

type TencentCOSUploader struct {
	MaxBytesPerSecond int64
}

func NewTencentCOSUploader(maxBytesPerSecond ...int64) TencentCOSUploader {
	uploader := TencentCOSUploader{}
	if len(maxBytesPerSecond) > 0 {
		uploader.MaxBytesPerSecond = maxBytesPerSecond[0]
	}
	return uploader
}

func (u TencentCOSUploader) Upload(ctx context.Context, request COSUploadRequest, progress ProgressReporter) (COSUploadResult, error) {
	if request.Region == "" || request.Bucket == "" || request.ObjectKey == "" || request.SourcePath == "" {
		return COSUploadResult{}, NewClassifiedError("PERMANENT", "cos upload request is incomplete")
	}
	if request.Secret.SecretID == "" || request.Secret.SecretKey == "" {
		return COSUploadResult{}, NewClassifiedError("AUTH", "cos credential is incomplete")
	}
	info, err := os.Stat(request.SourcePath)
	if err != nil {
		return COSUploadResult{}, NewClassifiedError("SOURCE_MISSING", fmt.Sprintf("cos upload source file is missing: %v", err))
	}
	if info.IsDir() {
		return COSUploadResult{}, NewClassifiedError("SOURCE_MISSING", "cos upload source path is a directory")
	}

	bucketURL, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", request.Bucket, request.Region))
	if err != nil {
		return COSUploadResult{}, NewClassifiedError("PERMANENT", fmt.Sprintf("invalid cos bucket endpoint: %v", err))
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &uploadProgressTransport{
			base: &cos.AuthorizationTransport{
				SecretID:     request.Secret.SecretID,
				SecretKey:    request.Secret.SecretKey,
				SessionToken: request.Secret.SessionToken,
			},
			ctx:               ctx,
			totalBytes:        info.Size(),
			maxBytesPerSecond: u.MaxBytesPerSecond,
			progress:          progress,
		},
	})
	response, err := client.Object.PutFromFile(ctx, request.ObjectKey, request.SourcePath, nil)
	if err != nil {
		return COSUploadResult{}, classifyCOSSDKError(err)
	}
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	etag := ""
	if response != nil {
		etag = strings.Trim(response.Header.Get("ETag"), `"`)
	}
	if progress != nil {
		progress(ctx, UploadProgress{CurrentBytes: info.Size(), TotalBytes: info.Size(), Message: "uploaded to cos"})
	}
	return COSUploadResult{ETag: etag, SizeBytes: info.Size()}, nil
}

type uploadProgressTransport struct {
	base              http.RoundTripper
	ctx               context.Context
	totalBytes        int64
	maxBytesPerSecond int64
	progress          ProgressReporter
}

func (t *uploadProgressTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.Method == http.MethodPut {
		req.Body = &progressReadCloser{
			ReadCloser:        req.Body,
			ctx:               t.ctx,
			totalBytes:        t.totalBytes,
			maxBytesPerSecond: t.maxBytesPerSecond,
			progress:          t.progress,
		}
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

type progressReadCloser struct {
	io.ReadCloser
	ctx               context.Context
	totalBytes        int64
	maxBytesPerSecond int64
	progress          ProgressReporter
	mu                sync.Mutex
	readBytes         int64
	startedAt         time.Time
	lastReportAt      time.Time
}

func (r *progressReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.observe(int64(n))
	}
	return n, err
}

func (r *progressReadCloser) observe(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if r.startedAt.IsZero() {
		r.startedAt = now
	}
	r.readBytes += n
	if r.maxBytesPerSecond > 0 {
		expected := time.Duration(float64(r.readBytes) / float64(r.maxBytesPerSecond) * float64(time.Second))
		if sleep := r.startedAt.Add(expected).Sub(now); sleep > 0 {
			timer := time.NewTimer(sleep)
			select {
			case <-r.ctx.Done():
			case <-timer.C:
			}
			timer.Stop()
		}
	}
	if r.progress == nil {
		return
	}
	if now.Sub(r.lastReportAt) < time.Second && r.readBytes < r.totalBytes {
		return
	}
	r.lastReportAt = now
	r.progress(r.ctx, UploadProgress{CurrentBytes: r.readBytes, TotalBytes: r.totalBytes, Message: "uploading to cos"})
}

func (TencentCOSUploader) SignedDownloadURL(ctx context.Context, request COSDownloadURLRequest, expiresIn time.Duration) (COSDownloadURLResult, error) {
	if request.Region == "" || request.Bucket == "" || request.ObjectKey == "" || expiresIn <= 0 {
		return COSDownloadURLResult{}, NewClassifiedError("PERMANENT", "cos download request is incomplete")
	}
	if request.Secret.SecretID == "" || request.Secret.SecretKey == "" {
		return COSDownloadURLResult{}, NewClassifiedError("AUTH", "cos credential is incomplete")
	}
	bucketURL, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", request.Bucket, request.Region))
	if err != nil {
		return COSDownloadURLResult{}, NewClassifiedError("PERMANENT", fmt.Sprintf("invalid cos bucket endpoint: %v", err))
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:     request.Secret.SecretID,
			SecretKey:    request.Secret.SecretKey,
			SessionToken: request.Secret.SessionToken,
		},
	})
	signedURL, err := client.Object.GetPresignedURL(ctx, http.MethodGet, request.ObjectKey, request.Secret.SecretID, request.Secret.SecretKey, expiresIn, nil)
	if err != nil {
		return COSDownloadURLResult{}, classifyCOSSDKError(err)
	}
	return COSDownloadURLResult{
		URL:       signedURL.String(),
		ExpiresAt: time.Now().UTC().Add(expiresIn).Format(time.RFC3339),
		ObjectKey: request.ObjectKey,
	}, nil
}

func classifyCOSSDKError(err error) error {
	if err == nil {
		return nil
	}
	if responseErr, ok := cos.IsCOSError(err); ok && responseErr != nil {
		code := responseErr.Code
		switch code {
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return NewClassifiedError("AUTH", err.Error())
		case "NoSuchBucket", "InvalidBucketName", "InvalidObjectName":
			return NewClassifiedError("PERMANENT", err.Error())
		default:
			if responseErr.Response != nil && responseErr.Response.StatusCode >= 500 {
				return NewClassifiedError("TRANSIENT", err.Error())
			}
		}
	}
	return NewClassifiedError("TRANSIENT", err.Error())
}
