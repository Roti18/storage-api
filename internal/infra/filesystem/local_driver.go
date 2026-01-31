package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"storages-api/internal/domain"
	"strings"
	"sync"
	"syscall"
)

// Comprehensive hidden/system/junk file filter
// Catches:
// 1. Starts with . (Linux), $ (Windows), ~ (Temp)
// 2. System folders: System Volume Information, RECYCLE, etc.
// 3. Dev junk: node_modules, vendor, .git, .idea, .vscode, dist, build, target, etc.
var hiddenFileRegex = regexp.MustCompile(`^([\.\$~])|(?i)(System Volume Information|RECYCLE|RECYCLER|desktop\.ini|thumbs\.db|node_modules|vendor|__pycache__|\.git|\.idea|\.vscode|\.dart_tool|\.pub-cache|\.svn|dist|build|target|obj|bin|\.cache|\.config|\.local|\.mozilla|\.rustup|\.cargo|\.npm)$`)

func isHiddenFile(name string) bool {
	// Fast path for common hidden files
	if len(name) > 0 && (name[0] == '.' || name[0] == '$' || name[0] == '~') {
		return true
	}
	return hiddenFileRegex.MatchString(name)
}

// Extensions for code/project files that should be ignored in Global Search/Recent/Index
var projectJunkExtensions = map[string]bool{
	// Code
	"c": true, "cpp": true, "h": true, "hpp": true, "cs": true, "go": true,
	"java": true, "js": true, "jsx": true, "ts": true, "tsx": true, "php": true,
	"py": true, "rb": true, "pl": true, "swift": true, "kt": true, "kts": true,
	"rs": true, "dart": true, "lua": true, "sh": true, "bat": true, "ps1": true,
	"cmd": true, "vb": true, "vbs": true, "sql": true, "r": true, "m": true,
	// Web / Config
	"html": true, "css": true, "scss": true, "less": true, "sass": true,
	"json": true, "xml": true, "yaml": true, "yml": true, "toml": true, "ini": true,
	"env": true, "lock": true, "mod": true, "sum": true, "map": true,
	"gitignore": true, "dockerignore": true,
	// Binary/Build
	"class": true, "jar": true, "war": true, "ear": true, "o": true, "obj": true,
	"dll": true, "so": true, "dylib": true, "exe": true, "bin": true, "dat": true,
	"log": true, "tmp": true, "bak": true, "swp": true,
}

func isProjectJunk(name string) bool {
	// Check exact filenames
	if name == "LICENSE" || name == "README" || name == "Makefile" {
		return true
	}

	// Check extensions
	ext := strings.ToLower(filepath.Ext(name))
	if len(ext) > 1 {
		return projectJunkExtensions[ext[1:]]
	}
	return false
}

type LocalDriver struct {
	Mounts map[string]string // storage name -> path
}

func NewLocalDriver(mounts map[string]string) *LocalDriver {
	return &LocalDriver{Mounts: mounts}
}

// Resolve storage name to root path (Case Insensitive)
func (d *LocalDriver) getStorageRoot(storageName string) (string, error) {
	storageName = strings.ToLower(storageName)
	for name, path := range d.Mounts {
		if strings.ToLower(name) == storageName {
			return filepath.Clean(path), nil
		}
	}
	return "", fmt.Errorf("storage '%s' not found", storageName)
}

// Validate path to ensure it doesn't escape the root
func (d *LocalDriver) validatePath(storageName, subPath string) (string, error) {
	rootPath, err := d.getStorageRoot(storageName)
	if err != nil {
		return "", err
	}

	// filepath.Join handles cleaning and stripping leading slashes
	fullPath := filepath.Join(rootPath, subPath)
	cleanPath := filepath.Clean(fullPath)

	// Security: Ensure the cleanPath is still within rootPath
	rel, err := filepath.Rel(rootPath, cleanPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid path: access outside root (rel:%s)", rel)
	}

	return cleanPath, nil
}

func (d *LocalDriver) ListStorages() []domain.StorageInfo {
	storages := make([]domain.StorageInfo, 0, len(d.Mounts))
	for name, path := range d.Mounts {
		total, used, free := d.getDiskUsage(path)
		isMounted := d.checkIfMounted(path)
		storages = append(storages, domain.StorageInfo{
			Name:      name,
			Path:      path,
			TotalSize: total,
			UsedSize:  used,
			FreeSize:  free,
			IsMounted: isMounted,
		})
	}
	return storages
}

func (d *LocalDriver) checkIfMounted(path string) bool {
	stat, err := os.Lstat(path)
	if err != nil {
		return false
	}

	parentStat, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return true // If we can't stat parent, assume it's root or something special
	}

	// If device ID is different from parent, it's a mount point
	// Note: This works on Linux/Unix
	return stat.Sys().(*syscall.Stat_t).Dev != parentStat.Sys().(*syscall.Stat_t).Dev
}

func (d *LocalDriver) getDiskUsage(path string) (total, used, free uint64) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		fmt.Printf("Error getting disk usage for %s: %v\n", path, err)
		return 0, 0, 0
	}

	// Total bytes
	fmt.Printf("DEBUG: Raw Statfs for %s: Blocks=%d, Bsize=%d\n", path, stat.Blocks, stat.Bsize)
	total = stat.Blocks * uint64(stat.Bsize)
	// Free bytes
	free = stat.Bfree * uint64(stat.Bsize)
	// Used bytes
	used = total - free

	return total, used, free
}

// READ: List directory contents
func (d *LocalDriver) ReadDir(storageName, subPath string, showHidden bool) ([]domain.FileInfo, error) {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	// Parallel processing for file info stats
	type fileResult struct {
		info domain.FileInfo
		err  error
	}

	maxWorkers := 16 // Tuned for HDD latency masking
	jobs := make(chan os.DirEntry, len(entries))
	results := make(chan fileResult, len(entries))
	var wg sync.WaitGroup

	// worker pool
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				name := entry.Name()
				// Filter hidden files
				// Regex: Start with non-alphabetic characters (dots, numbers, symbols, etc)
				// Unless it's just alphanumeric start, we consider it hidden if showHidden is false
				if !showHidden && isHiddenFile(name) {
					results <- fileResult{err: fmt.Errorf("skipped")} // Skip signal
					continue
				}

				info, err := entry.Info()
				if err != nil {
					results <- fileResult{err: err}
					continue
				}

				relPath := filepath.Join(subPath, name)
				isDir := info.IsDir()
				itemCount := 0
				if isDir {
					// Quick count of immediate children (not recursive)
					subEntries, _ := os.ReadDir(filepath.Join(fullPath, name))
					itemCount = len(subEntries)
				}

				results <- fileResult{
					info: domain.FileInfo{
						Name:      name,
						Size:      info.Size(),
						Mode:      info.Mode().String(),
						ModTime:   info.ModTime(),
						IsDir:     isDir,
						Extension: filepath.Ext(name),
						ItemCount: itemCount,
						Path:      relPath,
					},
				}
			}
		}()
	}

	// Dispatch jobs
	for _, entry := range entries {
		jobs <- entry
	}
	close(jobs)

	// Wait for workers in background to close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	files := make([]domain.FileInfo, 0, len(entries))
	for res := range results {
		if res.err == nil {
			files = append(files, res.info)
		}
	}

	return files, nil
}

// ReadDirRecursive: Recursive scan for all files (Used by indexer)
func (d *LocalDriver) ReadDirRecursive(storageName string, showHidden bool) ([]domain.FileInfo, error) {
	fmt.Printf("SCAN: Starting recursive scan for %s...\n", storageName)
	rootPath, err := d.getStorageRoot(storageName)
	if err != nil {
		return nil, err
	}

	var allFiles []domain.FileInfo
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == rootPath {
			return nil
		}

		name := info.Name()
		rel, _ := filepath.Rel(rootPath, path)

		// Hidden check
		if !showHidden {
			parts := strings.Split(rel, string(os.PathSeparator))
			for _, part := range parts {
				if isHiddenFile(part) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}

		// Filter Project/Code Junk from Index
		if !info.IsDir() && isProjectJunk(name) {
			return nil
		}

		allFiles = append(allFiles, domain.FileInfo{
			Name:      name,
			Size:      info.Size(),
			Mode:      info.Mode().String(),
			ModTime:   info.ModTime(),
			IsDir:     info.IsDir(),
			Extension: filepath.Ext(name),
			Path:      rel,
		})
		return nil
	})

	return allFiles, err
}

// SEARCH: Search files recursively with filter and pagination
func (d *LocalDriver) SearchFiles(storageName string, extensions []string, limit, offset int, showHidden bool) ([]domain.FileInfo, int, error) {
	rootPath, err := d.getStorageRoot(storageName)
	if err != nil {
		return nil, 0, err
	}

	// Prepare map for fast lookup
	extMap := make(map[string]bool)
	for _, ext := range extensions {
		extMap[strings.ToLower(ext)] = true
	}

	var results []domain.FileInfo
	totalMatches := 0
	skipped := 0

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// 1. Skip Root
		if path == rootPath {
			return nil
		}

		name := info.Name()

		// 2. Hidden Filter
		if !showHidden {
			// Check if any part of path is hidden
			rel, _ := filepath.Rel(rootPath, path)
			parts := strings.Split(rel, string(os.PathSeparator))
			for _, part := range parts {
				if isHiddenFile(part) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}

		if info.IsDir() {
			return nil // Continue walking but don't add folders to result
		}

		// 3. Extension Filter
		ext := strings.ToLower(filepath.Ext(name))
		// Remove dot for comparison if needed, but usually Ext keeps dot
		if len(ext) > 0 && ext[0] == '.' {
			ext = ext[1:]
		}

		if len(extensions) > 0 && !extMap[ext] {
			return nil
		}

		totalMatches++

		// 4. Pagination Logic
		if skipped < offset {
			skipped++
			return nil
		}

		if limit > 0 && len(results) >= limit {
			// Optimization: If we just need one page, we might want to stop?
			// But to get TOTAL count accurately we must continue walking.
			// However, walking 1TB disk just to count is slow.
			// Let's assume for Quick Access Count we have a separate lighter logic,
			// and for Listing we just return partial found so far?
			// User wants "Count" AND "View".
			// If this function is for VIEW (Pagination), we can stop.
			// But totalMatches will be wrong.
			// Let's separate Count and List or use a specialized walker?
			// For now, let's just Collect hits up to limit.
			// Wait, the user wants "COUNT" separately.
			// So this function is strictly for LISTING.
			// We can stop walking if we reached limit.
			return filepath.SkipAll // Go 1.20+
		}

		relPath, _ := filepath.Rel(rootPath, path)
		relPath = filepath.ToSlash(relPath)

		results = append(results, domain.FileInfo{
			Name:      name,
			Size:      info.Size(),
			Mode:      info.Mode().String(),
			ModTime:   info.ModTime(),
			IsDir:     false,
			Extension: filepath.Ext(name),
			Path:      relPath,
		})

		return nil
	})

	// Handle Go version compatibility or simply limit return
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, totalMatches, err
}

// COUNT Stats: Fast recursive count by extension
func (d *LocalDriver) CountByExtensions(storageName string, extGroups map[string][]string, showHidden bool) (map[string]int, error) {
	rootPath, err := d.getStorageRoot(storageName)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int)
	// Invert map for O(1) lookup: "jpg" -> "images"
	extToGroup := make(map[string]string)
	for group, exts := range extGroups {
		stats[group] = 0
		for _, e := range exts {
			extToGroup[strings.ToLower(e)] = group
		}
	}

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path == rootPath {
				return nil
			}
			if !showHidden && isHiddenFile(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !showHidden {
			// Optimization: Check only file name if parent was already checked?
			// filepath.Walk descends, so if parent was hidden we skipped dir.
			// So only check file name here.
			if isHiddenFile(info.Name()) {
				return nil
			}
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if len(ext) > 0 {
			ext = ext[1:]
		} // remove dot

		if group, ok := extToGroup[ext]; ok {
			stats[group]++
		}
		return nil
	})

	return stats, err
}

func (d *LocalDriver) CreateFolder(storageName, subPath string) error {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(fullPath, 0755)
}

func (d *LocalDriver) SaveFile(storageName, subPath string, src io.Reader) error {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return err
	}
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

func (d *LocalDriver) GetRealPath(storageName, subPath string) (string, error) {
	return d.validatePath(storageName, subPath)
}

func (d *LocalDriver) GetFile(storageName, subPath string) (*os.File, error) {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return nil, err
	}
	return os.Open(fullPath)
}

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

func (d *LocalDriver) Delete(storageName, subPath string) error {
	fullPath, err := d.validatePath(storageName, subPath)
	if err != nil {
		return err
	}
	return os.RemoveAll(fullPath)
}

func (d *LocalDriver) Copy(storageName, srcPath, dstPath string) error {
	srcFullPath, err := d.validatePath(storageName, srcPath)
	if err != nil {
		return err
	}
	dstFullPath, err := d.validatePath(storageName, dstPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(srcFullPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return d.copyDir(srcFullPath, dstFullPath)
	}
	return d.copyFile(srcFullPath, dstFullPath)
}

func (d *LocalDriver) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

func (d *LocalDriver) copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dst, info.Mode())
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = d.copyDir(srcPath, dstPath)
		} else {
			err = d.copyFile(srcPath, dstPath)
		}

		if err != nil {
			return err
		}
	}
	return nil
}

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
