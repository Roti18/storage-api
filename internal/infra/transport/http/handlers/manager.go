package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"storages-api/internal/app"
	"storages-api/internal/domain"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type FileManagerHandler struct {
	service *app.FilesystemService
}

func NewFileManagerHandler(service *app.FilesystemService) *FileManagerHandler {
	return &FileManagerHandler{service: service}
}

// GET /api/storages - List available storages
func (h *FileManagerHandler) ListStorages(c *fiber.Ctx) error {
	storages := h.service.ListStorages()
	return c.JSON(fiber.Map{
		"storages": storages,
	})
}

// GET /api/files?storage=ssd1&path=/some/folder
func (h *FileManagerHandler) ListFiles(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage parameter is required",
		})
	}

	path := c.Query("path", "/")
	recursive := c.Query("recursive") == "true"
	showHidden := c.Query("show_hidden") == "true"

	var files []domain.FileInfo
	var err error

	if recursive {
		files, err = h.service.ListAllFiles(storage, showHidden)
	} else {
		files, err = h.service.ListFiles(storage, path, showHidden)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"storage": storage,
		"path":    path,
		"files":   files,
	})
}

// POST /api/folder
func (h *FileManagerHandler) CreateFolder(c *fiber.Ctx) error {
	var req domain.CreateFolderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.Storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage is required",
		})
	}

	if err := h.service.CreateFolder(req.Storage, req.Path); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "folder created",
		"storage": req.Storage,
		"path":    req.Path,
	})
}

// POST /api/upload?storage=ssd1&path=/target/folder
func (h *FileManagerHandler) UploadFile(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage parameter is required",
		})
	}

	targetPath := c.Query("path", "/")

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "no file uploaded",
		})
	}

	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "failed to open uploaded file",
		})
	}
	defer src.Close()

	fullPath := filepath.Join(targetPath, file.Filename)

	if err := h.service.UploadFile(storage, fullPath, src); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(domain.UploadResponse{
		Success:  true,
		Message:  "file uploaded successfully",
		FilePath: fullPath,
	})
}

// GET /api/download?storage=ssd1&path=/some/file.txt
func (h *FileManagerHandler) DownloadFile(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage parameter is required",
		})
	}

	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "path is required",
		})
	}

	fullPath, err := h.service.GetRealPath(storage, path)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "file not found"})
	}

	file, err := os.Stat(fullPath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to stat file"})
	}

	// Force set Content-Length for faster downloads and progress tracking on mobile devices
	c.Set("Content-Length", fmt.Sprintf("%d", file.Size()))
	c.Set("Content-Disposition", "attachment; filename="+filepath.Base(path))

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		c.Set("Content-Type", "image/jpeg")
	case ".png":
		c.Set("Content-Type", "image/png")
	case ".mp4":
		c.Set("Content-Type", "video/mp4")
	case ".pdf":
		c.Set("Content-Type", "application/pdf")
	case ".txt":
		c.Set("Content-Type", "text/plain")
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	return c.SendFile(fullPath)
}

// GET /api/preview?storage=ssd&path=/image.jpg
func (h *FileManagerHandler) PreviewFile(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage parameter is required",
		})
	}

	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "path is required",
		})
	}

	fullPath, err := h.service.GetRealPath(storage, path)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "file not found",
		})
	}

	// Inline preview in browser
	c.Set("Content-Disposition", "inline; filename="+filepath.Base(path))

	// Auto-detect Content-Type
	ext := strings.ToLower(filepath.Ext(path))
	isThumb := c.Query("thumb") == "true"

	switch ext {
	case ".jpg", ".jpeg":
		c.Set("Content-Type", "image/jpeg")
	case ".png":
		c.Set("Content-Type", "image/png")
	case ".gif":
		c.Set("Content-Type", "image/gif")
	case ".webp":
		c.Set("Content-Type", "image/webp")
	case ".mp4", ".mkv", ".webm", ".mov", ".avi":
		if isThumb {
			// Video thumbnail generation
			thumb, err := h.service.GetVideoThumbnail(fullPath)
			if err == nil {
				c.Set("Content-Type", "image/jpeg")
				return c.Send(thumb)
			}
		}
		// Full video stream
		c.Set("Content-Type", "video/mp4")
	case ".mp3":
		c.Set("Content-Type", "audio/mpeg")
	case ".pdf":
		c.Set("Content-Type", "application/pdf")
	case ".txt":
		c.Set("Content-Type", "text/plain")
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	return c.SendFile(fullPath)
}

// GET /api/search?storage=ssd&ext=jpg,png&limit=40&offset=0
func (h *FileManagerHandler) SearchFiles(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{"error": "storage required"})
	}

	extParam := c.Query("ext")
	var extensions []string
	if extParam != "" {
		extensions = strings.Split(extParam, ",")
	}

	limit := c.QueryInt("limit", 0)
	offset := c.QueryInt("offset", 0)
	days := c.QueryInt("days", 0)

	files, total := h.service.SearchIndexedFiles(storage, extensions, limit, offset, days)

	return c.JSON(fiber.Map{
		"files":  files,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"days":   days,
	})
}

// GET /api/recent?storage=ssd&limit=50&offset=0
func (h *FileManagerHandler) GetRecent(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{"error": "storage required"})
	}

	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	files := h.service.GetRecentFiles(storage, limit, offset)

	return c.JSON(fiber.Map{
		"files":  files,
		"limit":  limit,
		"offset": offset,
	})
}

// GET /api/reindex
func (h *FileManagerHandler) Reindex(c *fiber.Ctx) error {
	go h.service.ReindexAll()
	return c.JSON(fiber.Map{
		"message": "Reindexing started in background",
	})
}

// POST /api/stats
// Body: { "photos": ["jpg","png"], "videos": ["mp4"] }
func (h *FileManagerHandler) GetStats(c *fiber.Ctx) error {
	var req map[string][]string
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{"error": "storage required"})
	}

	stats := make(map[string]int)
	totalFiles := 0
	sumKnown := 0

	// Get total file count first
	_, totalFiles = h.service.SearchIndexedFiles(storage, []string{}, 0, 0, 0)

	for category, exts := range req {
		if category == "others" {
			continue
		}
		_, count := h.service.SearchIndexedFiles(storage, exts, 0, 0, 0)
		stats[category] = count
		sumKnown += count
	}

	// Calculate others
	if _, ok := req["others"]; ok {
		stats["others"] = totalFiles - sumKnown
		if stats["others"] < 0 {
			stats["others"] = 0
		}
	}

	return c.JSON(fiber.Map{
		"stats": stats,
	})
}

// PUT /api/rename
func (h *FileManagerHandler) RenameOrMove(c *fiber.Ctx) error {
	var req domain.RenameRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.Storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage is required",
		})
	}

	if err := h.service.RenameOrMove(req.Storage, req.OldPath, req.NewPath); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "renamed/moved successfully",
	})
}

// DELETE /api/delete?storage=ssd1&path=/some/file
func (h *FileManagerHandler) Delete(c *fiber.Ctx) error {
	storage := c.Query("storage")
	if storage == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "storage parameter is required",
		})
	}

	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "path is required",
		})
	}

	if err := h.service.Delete(storage, path); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "deleted successfully",
		"storage": storage,
		"path":    path,
	})
}

// POST /api/copy
func (h *FileManagerHandler) Copy(c *fiber.Ctx) error {
	var req domain.RenameRequest // Reuse RenameRequest as it has storage, old_path (src), and new_path (dst)
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Storage == "" || req.OldPath == "" || req.NewPath == "" {
		return c.Status(400).JSON(fiber.Map{"error": "storage, old_path, and new_path are required"})
	}

	if err := h.service.Copy(req.Storage, req.OldPath, req.NewPath); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "copied successfully",
	})
}

// POST /api/duplicate
func (h *FileManagerHandler) Duplicate(c *fiber.Ctx) error {
	var req struct {
		Storage string `json:"storage"`
		Path    string `json:"path"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Storage == "" || req.Path == "" {
		return c.Status(400).JSON(fiber.Map{"error": "storage and path are required"})
	}

	if err := h.service.Duplicate(req.Storage, req.Path); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "duplicated successfully",
	})
}
