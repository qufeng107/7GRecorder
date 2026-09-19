package storagepolicy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const LiveAnalyticsRawBytes int64 = 2 * 1024 * 1024 * 1024

func DirectoryUsage(root string) (bytes, files int64, err error) {
	err = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := entry.Info()
		if errors.Is(infoErr, os.ErrNotExist) {
			return nil
		}
		if infoErr != nil {
			return infoErr
		}
		if info.Mode().IsRegular() {
			bytes += info.Size()
			files++
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	return bytes, files, err
}
