package songs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestACRCloudRecognizerSubmitsPollsAndParsesMusic(t *testing.T) {
	var submitted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/file-1" {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":{"id":"file-1","state":1,"results":{"music":[{"offset":12.5,"played_duration":42,"result":{"acrid":"acr-1","title":"Test Song","artists":[{"name":"Test Artist"}],"external_ids":{"isrc":"TEST123"},"score":98}}]}}}`)
				return
			}
			fmt.Fprint(w, `{"data":[]}`)
		case http.MethodPost:
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("data_type") != "audio" || r.FormValue("name") != "run.mp3" {
				t.Fatalf("unexpected form: %#v", r.MultipartForm.Value)
			}
			submitted.Store(true)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"data":{"file_id":"file-1","state":0}}`)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	audioPath := filepath.Join(t.TempDir(), "analysis.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	recognizer := ACRCloudRecognizer{Client: server.Client(), PollInterval: time.Millisecond, MaxPollTime: time.Second, BaseURL: server.URL}
	result, err := recognizer.Recognize(context.Background(), RecognitionRequest{
		Region: "eu-west-1", ContainerID: "1", AccessToken: "test-token", AudioPath: audioPath, ProviderFilename: "run.mp3",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !submitted.Load() || result.ProviderFileID != "file-1" || len(result.Matches) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	match := result.Matches[0]
	if match.Title != "Test Song" || match.Artist != "Test Artist" || match.StartMs != 12500 || match.EndMs != 54500 || match.ISRC != "TEST123" {
		t.Fatalf("unexpected match: %#v", match)
	}
}

func TestParseACRMatchesCoalescesAdjacentSameIdentity(t *testing.T) {
	matches, err := parseACRMatches([]byte(`{"music":[
		{"offset":0,"played_duration":20,"result":{"acrid":"same","title":"Song","artists":[{"name":"Artist"}],"score":90}},
		{"offset":30,"played_duration":20,"result":{"acrid":"same","title":"Song","artists":[{"name":"Artist"}],"score":95}}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].StartMs != 0 || matches[0].EndMs != 50000 || matches[0].Score != 95 {
		t.Fatalf("unexpected coalesced matches: %#v", matches)
	}
}

func TestACRCloudRecognizerClassifiesAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer server.Close()
	recognizer := ACRCloudRecognizer{Client: server.Client(), BaseURL: server.URL}
	_, err := recognizer.Recognize(context.Background(), RecognitionRequest{AccessToken: "bad", ProviderFilename: "run.mp3"}, nil)
	if ProviderErrorClass(err) != "AUTH" {
		t.Fatalf("expected AUTH, got %v (%v)", ProviderErrorClass(err), err)
	}
}

func TestRecognitionPollDelayBackoff(t *testing.T) {
	base := 30 * time.Second
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for index, expected := range want {
		if actual := recognitionPollDelay(base, index+1); actual != expected {
			t.Fatalf("poll %d delay=%s want=%s", index+1, actual, expected)
		}
	}
}
