package app

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"storages-api/internal/domain"
	"storages-api/internal/infra/filesystem"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type cacheEntry struct {
	files     []domain.FileInfo
	timestamp time.Time
}

type FilesystemService struct {
	driver *filesystem.LocalDriver
	cache  map[string]cacheEntry
	mu     sync.RWMutex

	// SQLite Indexing system
	db *sql.DB
}

func NewFilesystemService(driver *filesystem.LocalDriver) *FilesystemService {
	// Use 'file:' prefix for proper URI parameter support in sqlite3
	db, err := sql.Open("sqlite3", "file:storage_index.db?_journal_mode=WAL&_sync=NORMAL")
	if err != nil {
		log.Fatalf("CRITICAL: Failed to open SQLite: %v", err)
	}
	if db == nil {
		log.Fatal("CRITICAL: SQL handle is nil")
	}

	// Create table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			storage TEXT,
			name TEXT,
			path TEXT,
			is_dir BOOLEAN,
			size INTEGER,
			modified DATETIME,
			extension TEXT,
			item_count INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_storage ON files(storage);
		CREATE INDEX IF NOT EXISTS idx_extension ON files(extension);
		CREATE INDEX IF NOT EXISTS idx_modified ON files(modified);
		CREATE INDEX IF NOT EXISTS idx_is_dir ON files(is_dir);
		CREATE INDEX IF NOT EXISTS idx_storage_ext_mod ON files(storage, extension, modified);
		CREATE INDEX IF NOT EXISTS idx_storage_isdir ON files(storage, is_dir);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_path_storage ON files(storage, path);
	`)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to initialize schema: %v", err)
	}

	s := &FilesystemService{
		driver: driver,
		cache:  make(map[string]cacheEntry),
		db:     db,
	}
	// Start background indexer
	go s.StartIndexing()
	return s
}

// Background Indexer: Runs periodically to keep SQLite index fresh
func (s *FilesystemService) StartIndexing() {
	ticker := time.NewTicker(30 * time.Minute) // SQLite is persistent, can run less often
	defer ticker.Stop()

	// Initial Scan immediately
	s.ReindexAll()

	for range ticker.C {
		s.ReindexAll()
	}
}

func (s *FilesystemService) ReindexAll() {
	storages := s.driver.ListStorages()
	var wg sync.WaitGroup
	for _, st := range storages {
		wg.Add(1)
		go func(st domain.StorageInfo) {
			defer wg.Done()
			files, err := s.driver.ReadDirRecursive(st.Name, false)
			if err != nil {
				fmt.Printf("ERROR: Failed to scan storage %s: %v\n", st.Name, err)
				return
			}
			s.updateIndex(st.Name, files)
			fmt.Printf("Indexed %s: %d files to SQLite\n", st.Name, len(files))
		}(st)
	}
	wg.Wait()
}

func (s *FilesystemService) updateIndex(storage string, files []domain.FileInfo) {
	tx, err := s.db.Begin()
	if err != nil {
		fmt.Printf("Error starting transaction: %v\n", err)
		return
	}
	defer tx.Rollback() // Safety Rollback

	// For simplicity, we clear and re-insert.
	_, err = tx.Exec("DELETE FROM files WHERE storage = ?", storage)
	if err != nil {
		fmt.Printf("Error clearing index for %s: %v\n", storage, err)
		return
	}

	stmt, err := tx.Prepare("INSERT INTO files(storage, name, path, is_dir, size, modified, extension, item_count) VALUES(?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		fmt.Printf("Error preparing statement: %v\n", err)
		return
	}
	defer stmt.Close()

	for _, f := range files {
		ext := f.Extension
		if len(ext) > 0 && ext[0] == '.' {
			ext = ext[1:]
		}
		_, err = stmt.Exec(storage, f.Name, f.Path, f.IsDir, f.Size, f.ModTime, strings.ToLower(ext), f.ItemCount)
		if err != nil {
			// Skip single record error but log it
			continue
		}
	}

	err = tx.Commit()
	if err != nil {
		fmt.Printf("Error committing transaction for %s: %v\n", storage, err)
	}
}

// SEARCH from SQLite (Persistent & Fast)
func (s *FilesystemService) SearchIndexedFiles(storage string, extensions []string, limit, offset, days int) ([]domain.FileInfo, int) {
	if s.db == nil {
		return []domain.FileInfo{}, 0
	}

	// Pure content filter (Hide system/hidden noise)
	query := `SELECT name, path, is_dir, size, modified, extension, item_count 
              FROM files 
              WHERE storage = ? AND is_dir = 0 
              AND name NOT LIKE '.%' 
              AND name NOT LIKE '$%' 
              AND name NOT LIKE '~%'`
	args := []interface{}{storage}

	if len(extensions) > 0 {
		placeholders := make([]string, len(extensions))
		for i, ext := range extensions {
			placeholders[i] = "?"
			args = append(args, strings.ToLower(ext))
		}
		query += " AND extension IN (" + strings.Join(placeholders, ",") + ")"
	}

	if days > 0 {
		query += " AND modified > ?"
		// Use formatted string for safer SQLite comparison
		args = append(args, time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05"))
	}

	// Count total matches
	countQuery := strings.Replace(query, "name, path, is_dir, size, modified, extension, item_count", "COUNT(*)", 1)
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		fmt.Printf("Count error: %v\n", err)
		return []domain.FileInfo{}, 0
	}

	// Optimization: If limit is 0 and offset is 0, user likely only wants the total count.
	if limit <= 0 && offset <= 0 {
		return []domain.FileInfo{}, total
	}

	// Add limit and offset
	query += " ORDER BY modified DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		return []domain.FileInfo{}, 0
	}
	defer rows.Close()

	var results []domain.FileInfo
	for rows.Next() {
		var f domain.FileInfo
		var ext sql.NullString
		err := rows.Scan(&f.Name, &f.Path, &f.IsDir, &f.Size, &f.ModTime, &ext, &f.ItemCount)
		if err == nil {
			f.Extension = ext.String
			results = append(results, f)
		}
	}

	return results, total
}

func (s *FilesystemService) GetRecentFiles(storage string, limit, offset int) []domain.FileInfo {
	if s.db == nil {
		return []domain.FileInfo{}
	}

	query := `
		SELECT name, path, is_dir, size, modified, extension 
		FROM files 
		WHERE storage = ? AND is_dir = 0 
		AND name NOT LIKE '.%' 
		AND name NOT LIKE '$%' 
		AND name NOT LIKE '~%'
		ORDER BY modified DESC 
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, storage, limit, offset)
	if err != nil {
		fmt.Printf("Recent query error: %v\n", err)
		return []domain.FileInfo{}
	}
	defer rows.Close()

	var results []domain.FileInfo
	for rows.Next() {
		var f domain.FileInfo
		var ext sql.NullString
		err := rows.Scan(&f.Name, &f.Path, &f.IsDir, &f.Size, &f.ModTime, &ext)
		if err == nil {
			f.Extension = ext.String
			results = append(results, f)
		}
	}
	return results
}

const cacheTTL = 60 * time.Second

func (s *FilesystemService) getCache(key string) ([]domain.FileInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, found := s.cache[key]
	if !found {
		return nil, false
	}
	if time.Since(entry.timestamp) > cacheTTL {
		return nil, false
	}
	return entry.files, true
}

func (s *FilesystemService) setCache(key string, files []domain.FileInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = cacheEntry{
		files:     files,
		timestamp: time.Now(),
	}
}

func (s *FilesystemService) invalidateStorage(storage string) {
	s.mu.Lock()
	// Clear standard cache
	prefix := fmt.Sprintf("%s:", storage)
	for k := range s.cache {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(s.cache, k)
		}
	}
	s.mu.Unlock()

	// Trigger Reindex for this storage in background
	go func() {
		files, err := s.driver.ReadDirRecursive(storage, true)
		if err == nil {
			s.updateIndex(storage, files)
		}
	}()
}

func (s *FilesystemService) ListStorages() []domain.StorageInfo {
	return s.driver.ListStorages()
}

func (s *FilesystemService) ListFiles(storage, path string, showHidden bool) ([]domain.FileInfo, error) {
	cacheKey := fmt.Sprintf("%s:%s:%t", storage, path, showHidden)
	if files, hit := s.getCache(cacheKey); hit {
		return files, nil
	}

	files, err := s.driver.ReadDir(storage, path, showHidden)
	if err == nil {
		s.setCache(cacheKey, files)
	}
	return files, err
}

func (s *FilesystemService) ListAllFiles(storage string, showHidden bool) ([]domain.FileInfo, error) {
	cacheKey := fmt.Sprintf("%s:recursive:%t", storage, showHidden)
	if files, hit := s.getCache(cacheKey); hit {
		return files, nil
	}

	files, err := s.driver.ReadDirRecursive(storage, showHidden)
	if err == nil {
		s.setCache(cacheKey, files)
	}
	return files, err
}

func (s *FilesystemService) CreateFolder(storage, path string) error {
	err := s.driver.CreateFolder(storage, path)
	if err == nil {
		s.invalidateStorage(storage)
	}
	return err
}

func (s *FilesystemService) UploadFile(storage, path string, src io.Reader) error {
	err := s.driver.SaveFile(storage, path, src)
	if err == nil {
		s.invalidateStorage(storage)
	}
	return err
}

func (s *FilesystemService) GetRealPath(storage, path string) (string, error) {
	return s.driver.GetRealPath(storage, path)
}

func (s *FilesystemService) DownloadFile(storage, path string) (io.ReadCloser, error) {
	return s.driver.GetFile(storage, path)
}

func (s *FilesystemService) RenameOrMove(storage, oldPath, newPath string) error {
	err := s.driver.Rename(storage, oldPath, newPath)
	if err == nil {
		s.invalidateStorage(storage)
	}
	return err
}

func (s *FilesystemService) Copy(storage, srcPath, dstPath string) error {
	err := s.driver.Copy(storage, srcPath, dstPath)
	if err == nil {
		s.invalidateStorage(storage)
	}
	return err
}

func (s *FilesystemService) Duplicate(storage, srcPath string) error {
	// Generate new path: /path/to/file.txt -> /path/to/file_copy.txt
	// For folders: /path/to/folder -> /path/to/folder_copy
	dir := filepath.Dir(srcPath)
	base := filepath.Base(srcPath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	newPath := filepath.Join(dir, nameWithoutExt+"_copy"+ext)

	// Check if exists, if so add number
	counter := 1
	for {
		realPath, _ := s.driver.GetRealPath(storage, newPath)
		if _, err := os.Stat(realPath); os.IsNotExist(err) {
			break
		}
		newPath = filepath.Join(dir, fmt.Sprintf("%s_copy_%d%s", nameWithoutExt, counter, ext))
		counter++
	}

	err := s.driver.Copy(storage, srcPath, newPath)
	if err == nil {
		s.invalidateStorage(storage)
	}
	return err
}

func (s *FilesystemService) Delete(storage, path string) error {
	err := s.driver.Delete(storage, path)
	if err == nil {
		s.invalidateStorage(storage)
	}
	return err
}

func (s *FilesystemService) IsDirectory(storage, path string) (bool, error) {
	return s.driver.IsDir(storage, path)
}

func (s *FilesystemService) GetVideoThumbnail(realPath string) ([]byte, error) {
	// Extract 1 frame at 1 second
	cmd := exec.Command("ffmpeg", "-ss", "00:00:01", "-i", realPath, "-vframes", "1", "-f", "mjpeg", "-q:v", "5", "pipe:1")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Thumbnail error for %s: %v\n", realPath, err)
		return nil, err
	}
	return out.Bytes(), nil
}
