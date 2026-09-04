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
