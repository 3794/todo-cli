package tui

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// errChanged means the file changed between reading it and replacing it.
var errChanged = errors.New("file changed during write")

type fileStat struct {
	modTime time.Time
	size    int64
	missing bool
}

func statFile(path string) fileStat {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStat{missing: true}
	}
	return fileStat{modTime: fi.ModTime(), size: fi.Size()}
}

// writeAtomic replaces path with content via a temp file in the same
// directory and a rename. Right before the rename it re-reads the file and
// returns errChanged, without touching it, if it no longer equals expected.
func writeAtomic(path string, content, expected []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(name)
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return errChanged
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
