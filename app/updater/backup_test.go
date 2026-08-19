package updater

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupDataIncludesDataAndExcludesPreviousBackups(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte("config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "backups"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "backups", "old.tar.gz"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := BackupData(directory, "operation")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	tape := tar.NewReader(gzipReader)
	found := false
	for {
		header, err := tape.Next()
		if err != nil {
			break
		}
		if header.Name == "config.json" {
			found = true
		}
		if header.Name == "backups/old.tar.gz" {
			t.Fatal("previous backup was recursively archived")
		}
	}
	if !found {
		t.Fatal("config.json was not backed up")
	}
}
