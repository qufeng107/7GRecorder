package songs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyWithProgress(t *testing.T) {
	var destination bytes.Buffer
	var latest DownloadProgress
	written, err := copyWithProgress(context.Background(), &destination, bytes.NewBufferString("audio-video"), 11,
		func(_ context.Context, progress DownloadProgress) { latest = progress })
	if err != nil {
		t.Fatal(err)
	}
	if written != 11 || destination.String() != "audio-video" || latest.CurrentBytes != 11 || latest.TotalBytes != 11 {
		t.Fatalf("unexpected download result: written=%d body=%q progress=%#v", written, destination.String(), latest)
	}
}

func TestCopyWithProgressHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var destination bytes.Buffer
	_, err := copyWithProgress(ctx, &destination, bytes.NewBufferString("data"), 4, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if destination.Len() != 0 {
		t.Fatalf("cancelled copy wrote %d bytes", destination.Len())
	}
}

func TestVideoObjectExtension(t *testing.T) {
	if !isVideoObject("archive/SHOW.MP4") {
		t.Fatal("expected MP4 to be accepted")
	}
	if isVideoObject("archive/danmaku.xml") {
		t.Fatal("expected XML to be rejected")
	}
}

func TestDownloaderReusesAtomicallyPromotedRunSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(path, []byte("complete"), 0o640); err != nil {
		t.Fatal(err)
	}
	var message string
	err := (TencentCOSDownloader{}).Download(context.Background(), DownloadRequest{
		Region: "ap-shanghai", Bucket: "bucket", ObjectKey: "source.mp4", ExpectedSize: 8,
		Destination: path, SecretID: "id", SecretKey: "key",
	}, func(_ context.Context, progress DownloadProgress) { message = progress.Message })
	if err != nil {
		t.Fatal(err)
	}
	if message != "Reused downloaded source" {
		t.Fatalf("unexpected progress message: %q", message)
	}
}
