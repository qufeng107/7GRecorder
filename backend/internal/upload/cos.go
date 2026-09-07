package upload

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	RecordingProfileID int64
	Region             string
	Bucket             string
	ObjectKey          string
	SourcePath         string
	SourceSizeBytes    int64
	Secret             COSSecret
}

type COSUploadResult struct {
	ETag string
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
	ObjectKey  string `json:"object_key"`
}

type COSUploader interface {
	Upload(ctx context.Context, request COSUploadRequest) (COSUploadResult, error)
}

type TencentCOSUploader struct{}

func NewTencentCOSUploader() TencentCOSUploader {
	return TencentCOSUploader{}
}

func (TencentCOSUploader) Upload(ctx context.Context, request COSUploadRequest) (COSUploadResult, error) {
	if request.Region == "" || request.Bucket == "" || request.ObjectKey == "" || request.SourcePath == "" {
		return COSUploadResult{}, NewClassifiedError("PERMANENT", "cos upload request is incomplete")
	}
	if request.Secret.SecretID == "" || request.Secret.SecretKey == "" {
		return COSUploadResult{}, NewClassifiedError("AUTH", "cos credential is incomplete")
	}

	bucketURL, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", request.Bucket, request.Region))
	if err != nil {
		return COSUploadResult{}, NewClassifiedError("PERMANENT", fmt.Sprintf("invalid cos bucket endpoint: %v", err))
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:     request.Secret.SecretID,
			SecretKey:    request.Secret.SecretKey,
			SessionToken: request.Secret.SessionToken,
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
	return COSUploadResult{ETag: etag}, nil
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
