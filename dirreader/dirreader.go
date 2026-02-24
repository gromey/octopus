package dirreader

import (
	"encoding/hex"
	"hash"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileInfo represents file information including its absolute and relative paths, and the file's hash.
type FileInfo struct {
	fs.FileInfo        // Embedding the standard FileInfo struct from the os package.
	PathRel     string // Relative path of the file with respect to the root.
	Hash        string // Hash of the file's content (optional).
}

// Exec initializes a dirReader and starts reading files from the provided root directory.
// It supports filtering files by mask (e.g., extensions) and computing file hashes using the provided hash function.
//   - root: the root directory to start reading.
//   - hashFunc: function to compute a hash for file contents (can be nil if not needed).
//   - mask: list of file extensions to include or exclude based on the 'include' flag.
//   - include: if true, only include files matching the mask; if false, exclude them.
func Exec(root string, hashFunc func() hash.Hash, mask []string, include bool) ([]*FileInfo, error) {
	r := &dirReader{
		hashFunc: hashFunc,
		mask:     mask,
		include:  include,
	}

	// If no mask is provided, disable filtering by setting 'include' to false.
	if len(r.mask) == 0 {
		r.include = false
	}

	// Start reading the root directory.
	return r.readDirectory(root)
}

// dirReader holds the state for reading directories and files.
type dirReader struct {
	wg       sync.WaitGroup
	hashFunc func() hash.Hash
	mask     []string
	include  bool
}

// readDirectory reads the contents of a directory and processes its files and subdirectories.
func (r *dirReader) readDirectory(root string) ([]*FileInfo, error) {
	var fileInfos []*FileInfo

	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// If the entry is a directory or files based on the mask (include or exclude them).
		if entry.IsDir() || r.filterByMask(entry.Name()) {
			return nil
		}

		fi := &FileInfo{PathRel: path}

		if fi.FileInfo, err = entry.Info(); err != nil {
			return err
		}

		// If a hash function is provided, compute the file's hash.
		if r.hashFunc != nil {
			r.wg.Add(1)
			go r.computeHash(fi)
		}

		fileInfos = append(fileInfos, fi)

		return nil
	}); err != nil {
		return nil, err
	}

	r.wg.Wait() // Wait for all directory and file processing to complete.

	return fileInfos, nil
}

// filterByMask checks if the file name matches any of the provided extensions in the mask.
func (r *dirReader) filterByMask(name string) bool {
	for i := range r.mask {
		if strings.HasSuffix(name, r.mask[i]) {
			return !r.include
		}
	}
	return r.include
}

// computeHash computes the hash of the file content using the provided hash function.
func (r *dirReader) computeHash(fileInfo *FileInfo) {
	defer r.wg.Done() // Ensure the WaitGroup is decremented when done.

	file, err := os.Open(fileInfo.PathRel)
	if err != nil {
		slog.Error("Failed to open file", slog.Any("error", err))
		return
	}
	defer func() { _ = file.Close() }()

	h := r.hashFunc()
	if _, err = io.Copy(h, file); err != nil {
		slog.Error("Failed to compute hash", slog.Any("error", err))
		return
	}

	fileInfo.Hash = hex.EncodeToString(h.Sum(nil))
}

// DeleteEmptyDirectories walks the directory tree rooted at the given path and
// removes any directories that are completely empty.
// The function returns the first error encountered during traversal, directory reading, or deletion.
func DeleteEmptyDirectories(root string) error {
	// Walk the directory tree from bottom to top
	return filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip files and only process directories.
		if !info.IsDir() {
			return nil
		}

		// Check if the directory is empty.
		var dirEntries []os.DirEntry
		if dirEntries, err = os.ReadDir(path); err != nil {
			return err
		}

		// If the directory is empty remove it.
		if len(dirEntries) == 0 {
			if err = os.Remove(path); err != nil {
				return err
			}

			return filepath.SkipDir
		}

		return nil
	})
}
