package httpserver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsafeRelativePath = errors.New("unsafe relative path")

func ResolveWithinRoot(root string, relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", ErrUnsafeRelativePath
	}

	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || cleaned == ".." {
		return "", ErrUnsafeRelativePath
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootAbs, cleaned)
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrUnsafeRelativePath
	}
	return candidateAbs, nil
}
