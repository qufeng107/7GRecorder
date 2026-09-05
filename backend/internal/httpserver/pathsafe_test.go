package httpserver

import "testing"

func TestResolveWithinRootRejectsEscape(t *testing.T) {
	root := t.TempDir()
	tests := []string{
		"../secret",
		"..",
		"/absolute/path",
	}

	for _, tt := range tests {
		if _, err := ResolveWithinRoot(root, tt); err == nil {
			t.Fatalf("expected %q to be rejected", tt)
		}
	}
}

func TestResolveWithinRootAcceptsManagedPath(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveWithinRoot(root, "recordings/1/file.flv")
	if err != nil {
		t.Fatal(err)
	}
	if got == root {
		t.Fatalf("expected child path, got root")
	}
}

func TestProtectedMediaPathEscapesSegments(t *testing.T) {
	got := protectedMediaPath("recordings/1741048619-七宫筱野/file name.flv")
	want := "/_protected_media/recordings/1741048619-%E4%B8%83%E5%AE%AB%E7%AD%B1%E9%87%8E/file%20name.flv"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
