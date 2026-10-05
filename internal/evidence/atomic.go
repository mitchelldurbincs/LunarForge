package evidence

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite publishes a complete file by renaming a sibling temporary file.
func AtomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".publish-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func writeOnce(path string, data []byte) error {
	// The run directory is uniquely reserved by the runner. Refuse any subsequent
	// attempt to rewrite a completed record, including failed records.
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("evidence is immutable: %w", err)
	}
	if err := lock.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("evidence is immutable: %s", path)
	}
	return AtomicWrite(path, data)
}
