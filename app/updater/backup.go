package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func FilesystemFreeBytes(path string) (uint64, error) {
	var status syscall.Statfs_t
	if err := syscall.Statfs(path, &status); err != nil {
		return 0, err
	}
	return uint64(status.Bavail) * uint64(status.Bsize), nil
}

func CheckRegistry(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ghcr.io/v2/", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return fmt.Errorf("GHCR returned HTTP %d", response.StatusCode)
	}
	return nil
}

func BackupData(dataDir, operationID string) (string, error) {
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(backupDir, "update-"+operationID+".tar.gz")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	walkErr := filepath.WalkDir(dataDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(dataDir, current)
		if err != nil || relative == "." {
			return err
		}
		if relative == "backups" || strings.HasPrefix(relative, "backups"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		source, err := os.Open(current)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeTarErr := tarWriter.Close()
	closeGzipErr := gzipWriter.Close()
	closeFileErr := file.Close()
	if walkErr != nil {
		return "", walkErr
	}
	if closeTarErr != nil {
		return "", closeTarErr
	}
	if closeGzipErr != nil {
		return "", closeGzipErr
	}
	if closeFileErr != nil {
		return "", closeFileErr
	}
	return path, nil
}
