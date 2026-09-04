package db

import (
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

func TestMigrateCleanDatabase(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataRoot:   root,
		SQLitePath: filepath.Join(root, "7grecorder.db"),
		TempRoot:   filepath.Join(root, "temp"),
	}

	if err := Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
}
