package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"
)

type FileMetadata struct {
	OriginalPath string
	ResolvedPath string
	Name         string
	SizeBytes    int64
	ModifiedAt   time.Time
	IsSymlink    bool
}

func Inspect(path string) (FileMetadata, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return FileMetadata{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return FileMetadata{}, err
	}
	resolved := abs
	isSymlink := info.Mode()&os.ModeSymlink != 0
	if isSymlink {
		resolved, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return FileMetadata{}, err
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return FileMetadata{}, err
		}
	}
	if !info.Mode().IsRegular() {
		return FileMetadata{}, os.ErrInvalid
	}
	file, err := os.Open(resolved)
	if err != nil {
		return FileMetadata{}, err
	}
	_ = file.Close()
	return FileMetadata{
		OriginalPath: abs,
		ResolvedPath: resolved,
		Name:         filepath.Base(resolved),
		SizeBytes:    info.Size(),
		ModifiedAt:   info.ModTime().UTC(),
		IsSymlink:    isSymlink,
	}, nil
}

func SHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Unchanged(path string, size int64, modified time.Time) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Size() == size && info.ModTime().UTC().Equal(modified.UTC())
}
