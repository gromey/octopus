package dirreader

import (
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileInfo represents file information including its absolute and relative paths, and the file's hash.
type FileInfo struct {
	os.FileInfo        // Embedding the standard FileInfo struct from the os package.
	PathRel     string // Relative path of the file with respect to the root.
	Hash        string // Hash of the file's content (optional).
}

// Exec initializes a dirReader and starts reading files from the provided root directory.
// It supports filtering files by mask (e.g., extensions) and computing file hashes using the provided hash function.
//   - root: the root directory to start reading.
//   - hashFunc: function to compute a hash for file contents (can be nil if not needed).
//   - mask: list of file extensions to include or exclude based on the 'include' flag.
//   - include: if true, only include files matching the mask; if false, exclude them.
func Exec(root string, hashFunc func() hash.Hash, mask []string, include bool) ([]FileInfo, error) {
	rooter, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}

	r := &dirReader{
		fileChan:  make(chan FileInfo),
		errorChan: make(chan error),
		root:      rooter,
		hashFunc:  hashFunc,
		mask:      mask,
		include:   include,
	}

	// If no mask is provided, disable filtering by setting 'include' to false.
	if len(r.mask) == 0 {
		r.include = false
	}

	return r.readDirectoryConcurrent()
}

// dirReader holds the state for reading directories and files.
type dirReader struct {
	swg       sync.WaitGroup
	wg        sync.WaitGroup
	root      *os.Root
	fileChan  chan FileInfo
	errorChan chan error
	hashFunc  func() hash.Hash
	mask      []string
	include   bool
}

// readDirectoryConcurrent reads the root directory concurrently and returns a list of FileInfo.
// It spawns goroutines to read files and collect results/errors.
func (r *dirReader) readDirectoryConcurrent() ([]FileInfo, error) {
	var fileInfos []FileInfo
	var err error

	// Goroutine to collect FileInfo results.
	r.swg.Add(1)
	go func() {
		for fi := range r.fileChan {
			fileInfos = append(fileInfos, fi)
		}
		r.swg.Done()
	}()

	// Goroutine to collect and aggregate errors.
	r.swg.Add(1)
	go func() {
		for e := range r.errorChan {
			err = errors.Join(err, e)
		}
		r.swg.Done()
	}()

	// Start reading the root directory.
	r.wg.Add(1)
	go r.readDirectory(".", "")
	r.wg.Wait() // Wait for all directory and file processing to complete.

	// Close the channels after processing is done.
	close(r.fileChan)
	close(r.errorChan)

	r.swg.Wait() // Wait for result/error collection to finish.

	if err != nil {
		return nil, err
	}

	return fileInfos, nil
}

// readDirectory reads the contents of a directory and processes its files and subdirectories.
func (r *dirReader) readDirectory(root, rel string) {
	defer r.wg.Done() // Ensure the WaitGroup is decremented when done.

	dir, err := r.root.Open(root)
	if err != nil {
		r.errorChan <- fmt.Errorf("open %s: %w", root, err)
		return
	}
	defer func() { _ = dir.Close() }()

	// Read all directory entries.
	var files []os.FileInfo
	if files, err = dir.Readdir(-1); err != nil {
		r.errorChan <- fmt.Errorf("read dir %s: %w", root, err)
		return
	}

	// Iterate over all files and directories in the current directory.
	for _, file := range files {
		if file.IsDir() {
			// If the entry is a directory, recursively read its contents.
			r.wg.Add(1)
			go r.readDirectory(filepath.Join(root, file.Name()), filepath.Join(rel, file.Name()))
			continue
		}

		// Filter files based on the mask (include or exclude them).
		if r.include != r.includedInMask(file.Name()) {
			continue
		}

		r.wg.Add(1)
		go r.getFileInfo(rel, file)
	}
}

// includedInMask checks if the file name matches any of the provided extensions in the mask.
func (r *dirReader) includedInMask(name string) bool {
	for _, ext := range r.mask {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// getFileInfo processes an individual file, optionally computes its hash.
func (r *dirReader) getFileInfo(rel string, file os.FileInfo) {
	defer r.wg.Done() // Ensure the WaitGroup is decremented when done.

	fi := FileInfo{
		FileInfo: file,
		PathRel:  rel,
	}

	// If a hash function is provided, compute the file's hash.
	if r.hashFunc != nil {
		var err error
		rel = filepath.Join(rel, file.Name())
		if fi.Hash, err = r.computeHash(rel); err != nil {
			r.errorChan <- fmt.Errorf("calculate hash sum %s: %w", rel, err)
		}
	}

	r.fileChan <- fi
}

// computeHash computes the hash of the file content using the provided hash function.
func (r *dirReader) computeHash(filename string) (string, error) {
	file, err := r.root.Open(filename)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	h := r.hashFunc()
	if _, err = io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// DeleteEmptyDirectories walks the directory tree rooted at the given path and
// removes any directories that are completely empty.
// The function returns the first error encountered during traversal, directory reading, or deletion.
func DeleteEmptyDirectories(root string) error {
	// Walk the directory tree from bottom to top
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip files and only process directories
		if !info.IsDir() {
			return nil
		}

		// Check if the directory is empty
		var dirEntries []os.DirEntry
		if dirEntries, err = os.ReadDir(path); err != nil {
			return err
		}

		// If the directory is empty, delete it
		if len(dirEntries) == 0 {
			if err = os.Remove(path); err != nil {
				return err
			}
		}

		return nil
	})
}
