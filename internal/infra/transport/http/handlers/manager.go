package handlers

import (
	"path/filepath"
	"storages-api/internal/app"
	"storages-api/internal/domain"

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

	files, err := h.service.ListFiles(storage, path)
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

	file, err := h.service.DownloadFile(storage, path)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "file not found",
		})
	}
	// JANGAN defer file.Close() - SendStream akan handle close otomatis

	// Force download
	c.Set("Content-Disposition", "attachment; filename="+filepath.Base(path))

	// Auto-detect Content-Type berdasarkan extension
	ext := filepath.Ext(path)
	switch ext {
	case ".jpg", ".jpeg":
		c.Set("Content-Type", "image/jpeg")
	case ".png":
		c.Set("Content-Type", "image/png")
	case ".gif":
		c.Set("Content-Type", "image/gif")
	case ".webp":
		c.Set("Content-Type", "image/webp")
	case ".mp4":
		c.Set("Content-Type", "video/mp4")
	case ".webm":
		c.Set("Content-Type", "video/webm")
	case ".mp3":
		c.Set("Content-Type", "audio/mpeg")
	case ".pdf":
		c.Set("Content-Type", "application/pdf")
	case ".txt":
		c.Set("Content-Type", "text/plain")
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	return c.SendStream(file)
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

	file, err := h.service.DownloadFile(storage, path)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "file not found",
		})
	}

	// Preview inline di browser
	c.Set("Content-Disposition", "inline; filename="+filepath.Base(path))

	// Auto-detect Content-Type
	ext := filepath.Ext(path)
	switch ext {
	case ".jpg", ".jpeg":
		c.Set("Content-Type", "image/jpeg")
	case ".png":
		c.Set("Content-Type", "image/png")
	case ".gif":
		c.Set("Content-Type", "image/gif")
	case ".webp":
		c.Set("Content-Type", "image/webp")
	case ".mp4":
		c.Set("Content-Type", "video/mp4")
	case ".webm":
		c.Set("Content-Type", "video/webm")
	case ".mp3":
		c.Set("Content-Type", "audio/mpeg")
	case ".pdf":
		c.Set("Content-Type", "application/pdf")
	case ".txt":
		c.Set("Content-Type", "text/plain")
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	return c.SendStream(file)
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
