package main

import (
	"fmt"
	"log"
	app2 "storages-api/internal/app"
	"storages-api/internal/config"
	"storages-api/internal/infra/filesystem"
	"storages-api/internal/infra/transport/http/handlers"
	"storages-api/internal/infra/transport/http/middleware"

	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	// Load Config
	cfg := config.LoadConfig()

	fmt.Printf("DEBUG: Loaded %d storage mounts\n", len(cfg.StorageMounts))
	for k, v := range cfg.StorageMounts {
		fmt.Printf("DEBUG: Storage [%s] -> Path [%s]\n", k, v)
	}

	// Init Fiber App
	app := fiber.New(fiber.Config{
		AppName:      "Storage API File Manager v1",
		BodyLimit:    100 * 1024 * 1024, // 100MB max upload
		ServerHeader: "StorageAPI",
	})

	// Middleware
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("startTime", time.Now())
		return c.Next()
	})
	app.Use(logger.New())
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed, // Optimize for speed
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Init Dependencies
	driver := filesystem.NewLocalDriver(cfg.StorageMounts)
	service := app2.NewFilesystemService(driver)
	fileHandler := handlers.NewFileManagerHandler(service)
	authHandler := handlers.NewAuthHandler(cfg)

	// Routes
	api := app.Group("/api")

	// Public - only login is public
	api.Post("/login", authHandler.Login)

	// Protected - all file operations require auth
	protected := api.Use(middleware.AuthMiddleware(cfg))

	// READ
	protected.Get("/files", fileHandler.ListFiles)       // List files/folders
	protected.Get("/preview", fileHandler.PreviewFile)   // Preview file (inline)
	protected.Get("/download", fileHandler.DownloadFile) // Download file (force download)

	// CREATE
	protected.Post("/folder", fileHandler.CreateFolder) // Create new folder
	protected.Post("/upload", fileHandler.UploadFile)   // Upload file

	// UPDATE
	protected.Put("/rename", fileHandler.RenameOrMove)  // Rename or move file/folder
	protected.Post("/copy", fileHandler.Copy)           // Copy file/folder
	protected.Post("/duplicate", fileHandler.Duplicate) // Duplicate file/folder

	// DELETE
	protected.Delete("/delete", fileHandler.Delete) // Delete file/folder

	protected.Get("/search", fileHandler.SearchFiles)
	protected.Get("/recent", fileHandler.GetRecent)
	protected.Get("/reindex", fileHandler.Reindex)
	protected.Post("/stats", fileHandler.GetStats)

	// Root endpoint - List available storages (also protected)
	protected.Get("/", fileHandler.ListStorages)

	// Health check
	app.Get("/ping", func(c *fiber.Ctx) error {
		startTime := c.Locals("startTime").(time.Time)
		latency := time.Since(startTime).String()
		return c.JSON(fiber.Map{
			"status":  "ok",
			"latency": latency,
			"mounts":  cfg.StorageMounts,
			"message": "pong",
		})
	})

	// Start Server
	fmt.Printf("Server running on port %s\n", cfg.Port)
	fmt.Printf("Managing %d storage(s):\n", len(cfg.StorageMounts))
	for name, path := range cfg.StorageMounts {
		fmt.Printf("   - %s: %s\n", name, path)
	}
	log.Fatal(app.Listen(":" + cfg.Port))
}
