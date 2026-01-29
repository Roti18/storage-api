package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"storages-api/internal/domain"
	"strings"
)

type LocalDriver struct {
	Mounts map[string]string // storage name -> path
}

func NewLocalDriver(mounts map[string]string) *LocalDriver {
	return &LocalDriver{Mounts: mounts}
}

// Resolve storage name ke root path
func (d *LocalDriver) getStorageRoot(storageName string) (string, error) {
	root, exists := d.Mounts[storageName]
	if !exists {
		return "", fmt.Errorf("storage '%s' not found", storageName)
	}
	return root, nil
}

// Validasi path biar gak keluar dari root
func (d *LocalDriver) validatePath(storageName, subPath string) (string, error) {
	root, err := d.getStorageRoot(storageName)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(root, subPath)
	cleanPath := filepath.Clean(fullPath)

	if !strings.HasPrefix(cleanPath, root) {
		return "", fmt.Errorf("invalid path: trying to access outside root directory")
	}

	return cleanPath, nil
}

// List available storages
func (d *LocalDriver) ListStorages() []string {
	storages := make([]string, 0, len(d.Mounts))
	for name := range d.Mounts {
		storages = append(storages, name)
	}
	return storages
}

// READ: List isi folder
func (d *LocalDriver) ReadDir(storageName, subPath string) ([]domain.FileInfo, error) {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var files []domain.FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, domain.FileInfo{
			Name:      entry.Name(),
			Size:      info.Size(),
			Mode:      info.Mode().String(),
			ModTime:   info.ModTime(),
			IsDir:     entry.IsDir(),
			Extension: filepath.Ext(entry.Name()),
			Path:      filepath.Join(subPath, entry.Name()),
		})
	}

	return files, nil
}

// CREATE: Bikin folder baru
func (d *LocalDriver) CreateFolder(storageName, subPath string) error {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return err
	}

	return os.MkdirAll(fullPath, 0755)
}

// CREATE: Upload file
func (d *LocalDriver) SaveFile(storageName, subPath string, src io.Reader) error {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return err
	}

	// Pastikan folder parent ada
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dst, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// READ: Download file
func (d *LocalDriver) GetFile(storageName, subPath string) (*os.File, error) {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return nil, err
	}

	return os.Open(fullPath)
}

// UPDATE: Rename/Move file atau folder (dalam satu storage)
func (d *LocalDriver) Rename(storageName, oldPath, newPath string) error {
	oldFullPath, err := d.validatePath(storageName, oldPath)
	if err != nil {
		return err
	}

	newFullPath, err := d.validatePath(storageName, newPath)
	if err != nil {
		return err
	}

	return os.Rename(oldFullPath, newFullPath)
}

// DELETE: Hapus file atau folder
func (d *LocalDriver) Delete(storageName, subPath string) error {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return err
	}

	return os.RemoveAll(fullPath)
}

// Cek apakah path adalah directory
func (d *LocalDriver) IsDir(storageName, subPath string) (bool, error) {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return false, err
	}

	return info.IsDir(), nil
}
