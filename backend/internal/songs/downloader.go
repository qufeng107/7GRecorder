package songs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type DownloadProgress struct {
	CurrentBytes int64
	TotalBytes   int64
	Message      string
}

type ProgressReporter func(context.Context, DownloadProgress)

type SourceDownloader interface {
	Download(context.Context, DownloadRequest, ProgressReporter) error
}

type TencentCOSDownloader struct{}

func (TencentCOSDownloader) Download(ctx context.Context, req DownloadRequest, progress ProgressReporter) error {
	if req.Region == "" || req.Bucket == "" || req.ObjectKey == "" || req.Destination == "" || req.ExpectedSize <= 0 {
		return NewDownloadError("PERMANENT", "COS download request is incomplete")
	}
	if req.SecretID == "" || req.SecretKey == "" {
		return NewDownloadError("AUTH", "COS credential is incomplete")
	}
	if info, err := os.Stat(req.Destination); err == nil && !info.IsDir() && info.Size() == req.ExpectedSize {
		if progress != nil {
			progress(ctx, DownloadProgress{CurrentBytes: req.ExpectedSize, TotalBytes: req.ExpectedSize, Message: "Reused downloaded source"})
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return NewDownloadError("PERMANENT", err.Error())
	}
	bucketURL, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", req.Bucket, req.Region))
	if err != nil {
		return NewDownloadError("PERMANENT", err.Error())
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{Transport: &cos.AuthorizationTransport{
		SecretID: req.SecretID, SecretKey: req.SecretKey, SessionToken: req.SessionToken,
	}})
	response, err := client.Object.Get(ctx, req.ObjectKey, nil)
	if err != nil {
		return classifyDownloadError(err)
	}
	defer response.Body.Close()
	etag := strings.Trim(response.Header.Get("ETag"), `"`)
	if req.ExpectedETag != "" && strings.Trim(req.ExpectedETag, `"`) != etag {
		return NewDownloadError("SOURCE_MISSING", "COS object ETag changed after analysis was requested")
	}
	if err := os.MkdirAll(filepath.Dir(req.Destination), 0o755); err != nil {
		return NewDownloadError("PERMANENT", err.Error())
	}
	partPath := req.Destination + ".part"
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return NewDownloadError("PERMANENT", err.Error())
	}
	written, copyErr := copyWithProgress(ctx, file, response.Body, req.ExpectedSize, progress)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written != req.ExpectedSize {
		_ = os.Remove(partPath)
		if copyErr != nil {
			return NewDownloadError("TRANSIENT", copyErr.Error())
		}
		if closeErr != nil {
			return NewDownloadError("PERMANENT", closeErr.Error())
		}
		return NewDownloadError("SOURCE_MISSING", fmt.Sprintf("COS object size changed: expected %d, received %d", req.ExpectedSize, written))
	}
	if err := os.Rename(partPath, req.Destination); err != nil {
		_ = os.Remove(partPath)
		return NewDownloadError("PERMANENT", err.Error())
	}
	if progress != nil {
		progress(ctx, DownloadProgress{CurrentBytes: written, TotalBytes: req.ExpectedSize, Message: "Downloaded source from COS"})
	}
	return nil
}

type DownloadError struct {
	Class   string
	Message string
}

func (e DownloadError) Error() string { return e.Message }
func NewDownloadError(class, message string) error {
	return DownloadError{Class: class, Message: message}
}
func DownloadErrorClass(err error) string {
	if typed, ok := err.(DownloadError); ok {
		return typed.Class
	}
	return "TRANSIENT"
}

func classifyDownloadError(err error) error {
	if responseErr, ok := cos.IsCOSError(err); ok && responseErr != nil {
		switch responseErr.Code {
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return NewDownloadError("AUTH", err.Error())
		case "NoSuchKey", "NoSuchBucket":
			return NewDownloadError("SOURCE_MISSING", err.Error())
		}
	}
	return NewDownloadError("TRANSIENT", err.Error())
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, progress ProgressReporter) (int64, error) {
	buffer := make([]byte, 256*1024)
	var written int64
	var lastProgressAt time.Time
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			count, writeErr := dst.Write(buffer[:n])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != n {
				return written, io.ErrShortWrite
			}
			if progress != nil && (lastProgressAt.IsZero() || time.Since(lastProgressAt) >= time.Second || written == total) {
				progress(ctx, DownloadProgress{CurrentBytes: written, TotalBytes: total, Message: "Downloading source from COS"})
				lastProgressAt = time.Now()
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
