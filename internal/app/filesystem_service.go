package app

import (
	"io"
	"storages-api/internal/domain"
	"storages-api/internal/infra/filesystem"
)

type FilesystemService struct {
	driver *filesystem.LocalDriver
}

func NewFilesystemService(driver *filesystem.LocalDriver) *FilesystemService {
	return &FilesystemService{driver: driver}
}

func (s *FilesystemService) ListStorages() []string {
	return s.driver.ListStorages()
}

func (s *FilesystemService) ListFiles(storage, path string) ([]domain.FileInfo, error) {
	return s.driver.ReadDir(storage, path)
}

func (s *FilesystemService) CreateFolder(storage, path string) error {
	return s.driver.CreateFolder(storage, path)
}

func (s *FilesystemService) UploadFile(storage, path string, src io.Reader) error {
	return s.driver.SaveFile(storage, path, src)
}

func (s *FilesystemService) DownloadFile(storage, path string) (io.ReadCloser, error) {
	return s.driver.GetFile(storage, path)
}

func (s *FilesystemService) RenameOrMove(storage, oldPath, newPath string) error {
	return s.driver.Rename(storage, oldPath, newPath)
}

func (s *FilesystemService) Delete(storage, path string) error {
	return s.driver.Delete(storage, path)
}

func (s *FilesystemService) IsDirectory(storage, path string) (bool, error) {
	return s.driver.IsDir(storage, path)
}
