package domain

import "time"

type FileInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Mode      string    `json:"mode"`
	ModTime   time.Time `json:"mod_time"`
	IsDir     bool      `json:"is_dir"`
	Extension string    `json:"extension"`
	ItemCount int       `json:"item_count"`
	Path      string    `json:"path"`
}

type CreateFolderRequest struct {
	Storage string `json:"storage"`
	Path    string `json:"path"`
}

type RenameRequest struct {
	Storage string `json:"storage"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

type DeleteRequest struct {
	Storage string `json:"storage"`
	Path    string `json:"path"`
}

type UploadResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	FilePath string `json:"file_path"`
}

type StorageInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	TotalSize uint64 `json:"total_size"`
	UsedSize  uint64 `json:"used_size"`
	FreeSize  uint64 `json:"free_size"`
	IsMounted bool   `json:"is_mounted"`
}
