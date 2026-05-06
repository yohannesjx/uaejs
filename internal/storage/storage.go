package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type ObjectStore interface {
	// Upload stores an object and returns the public URL or relative path
	Upload(ctx context.Context, tenantID, fileID, ext string, r io.Reader) (string, error)
	// Delete removes an object
	Delete(ctx context.Context, url string) error
}

type LocalStore struct {
	baseDir string
	baseURL string
}

func NewLocalStore(baseDir, baseURL string) (*LocalStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &LocalStore{baseDir: baseDir, baseURL: baseURL}, nil
}

func (s *LocalStore) Upload(ctx context.Context, tenantID, fileID, ext string, r io.Reader) (string, error) {
	filename := fmt.Sprintf("%s_%s%s", tenantID, fileID, ext)
	path := filepath.Join(s.baseDir, filename)

	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s", s.baseURL, filename), nil
}

func (s *LocalStore) Delete(ctx context.Context, url string) error {
	filename := filepath.Base(url)
	path := filepath.Join(s.baseDir, filename)
	// ignore remove errors if file is already deleted
	os.Remove(path)
	return nil
}
