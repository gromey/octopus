package zipper

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ZipSrcDir returns a writer function that walks the directory tree rooted at
// src and adds all files and subdirectories into the provided zip.Writer. The
// relative paths of files are preserved, and the resulting entries are written
// to the ZIP archive. The root directory itself and the destination ZIP file
// (if inside the tree) are skipped.
func ZipSrcDir(src string, dest string) func(context.Context, *zip.Writer) error {
	return func(ctx context.Context, zipWriter *zip.Writer) error {
		// Walk through the source path to gather files
		return filepath.WalkDir(src, func(path string, info os.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("error walking the file tree: %w", err)
			}

			// Skip the root directory and the zip file itself
			if path == src || path == dest {
				return nil
			}

			// Add the file to the zip archive
			return addFileToZip(ctx, zipWriter, path, path)
		})
	}
}

// ZipSrcMap returns a writer function that adds the files specified in the src
// map into the provided zip.Writer. Keys represent absolute paths to files,
// while values represent the directory structure within the ZIP archive. The
// function ensures directories are created inside the archive before writing
// their corresponding files.
func ZipSrcMap(src map[string]string) func(context.Context, *zip.Writer) error {
	return func(ctx context.Context, zipWriter *zip.Writer) error {
		for file, rel := range src {
			if !strings.HasSuffix(rel, "/") {
				rel += "/"
			}

			if err := createFolder(zipWriter, rel); err != nil {
				return err
			}

			rel += filepath.Base(file)

			// Add the file to the zip archive
			if err := addFileToZip(ctx, zipWriter, file, rel); err != nil {
				return err
			}
		}

		return nil
	}
}

// ZipSrcData returns a writer function that writes the provided data as a single
// file in the ZIP archive using the given filename. A file header is created
// with the current modification time, and the resulting file entry is written
// to the provided zip.Writer.
func ZipSrcData(filename string, data []byte) func(context.Context, *zip.Writer) error {
	return func(ctx context.Context, zipWriter *zip.Writer) error {
		// Create a file header with the current time
		header := &zip.FileHeader{
			Name:     filename,
			Method:   zip.Deflate,
			Modified: time.Now(),
		}

		// Create a file inside the ZIP with the correct header
		file, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// Write data into the ZIP file
		if _, err = file.Write(data); err != nil {
			return err
		}

		return nil
	}
}

// ZipToFile creates the destination ZIP file on disk and invokes the provided writer function to populate it.
func ZipToFile(ctx context.Context, writer func(context.Context, *zip.Writer) error, dest string) error {
	dir := filepath.Dir(dest)

	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create a directory for the %s file: %w", dest, err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer closer(ctx, root)

	// Create the zip file
	var zipFile *os.File
	if zipFile, err = root.Create(filepath.Base(dest)); err != nil {
		return err
	}
	defer closer(ctx, zipFile)

	// Create a new zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer closer(ctx, zipWriter)

	return writer(ctx, zipWriter)
}

// ZipInMemory creates a ZIP archive entirely in memory and returns the resulting bytes.Buffer.
// The provided writer function is invoked to populate the archive.
func ZipInMemory(ctx context.Context, writer func(context.Context, *zip.Writer) error) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)

	zipWriter := zip.NewWriter(buf)
	defer closer(ctx, zipWriter)

	if err := writer(ctx, zipWriter); err != nil {
		return nil, err
	}

	return buf, nil
}

func createFolder(zipWriter *zip.Writer, path string) error {
	folders := strings.Split(path, "/")
	path = ""
	for _, folder := range folders {
		if folder == "" {
			return nil
		}
		folder = path + folder + "/"
		if _, err := zipWriter.CreateHeader(&zip.FileHeader{
			Name:     folder,
			Modified: time.Now(),
		}); err != nil {
			return err
		}
		path = folder
	}
	return nil
}

func addFileToZip(ctx context.Context, zipWriter *zip.Writer, filePath, zipPath string) error {
	root, err := os.OpenRoot(filepath.Dir(filePath))
	if err != nil {
		return err
	}
	defer closer(ctx, root)

	// Open the file for reading
	var file *os.File
	if file, err = root.Open(filepath.Base(filePath)); err != nil {
		return err
	}
	defer closer(ctx, file)

	// Get file info for the original file
	var info os.FileInfo
	if info, err = file.Stat(); err != nil {
		return err
	}

	if info.IsDir() {
		zipPath += "/"
	}

	// Create a zip file header
	header, _ := zip.FileInfoHeader(info)
	header.Name = zipPath
	header.Method = zip.Deflate

	// Create the writer for this file in the zip archive
	var writer io.Writer
	if writer, err = zipWriter.CreateHeader(header); err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	// Copy the file's content to the zip writer
	_, err = io.Copy(writer, file)
	return err
}

// Unzip extracts the contents of the ZIP file at src into the directory dest.
// The function enforces a maximum file size for entries, returning an error if any file exceeds maxFileSize.
// Directories and files are recreated with their original permissions.
// Any extraction or filesystem error is returned.
func Unzip(ctx context.Context, src string, dest string, maxFileSize int64) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer closer(ctx, r)

	dest = filepath.Clean(dest)
	if err = os.MkdirAll(dest, 0750); err != nil {
		return err
	}

	var root *os.Root
	if root, err = os.OpenRoot(dest); err != nil {
		return err
	}
	defer closer(ctx, root)

	for _, file := range r.File {
		if err = unzipFile(ctx, file, root, maxFileSize); err != nil {
			return err
		}
	}

	return nil
}

func unzipFile(ctx context.Context, file *zip.File, root *os.Root, maxFileSize int64) error {
	size := file.FileInfo().Size()

	if size > maxFileSize {
		return fmt.Errorf("file %s too large: %d bytes", file.Name, size)
	}

	// Create a directory if the file is dir
	if file.FileInfo().IsDir() {
		return root.MkdirAll(file.Name, 0750)
	}

	err := root.MkdirAll(filepath.Dir(file.Name), 0750)
	if err != nil {
		return err
	}

	// Create the file
	var out *os.File
	if out, err = root.Create(file.Name); err != nil {
		return err
	}
	defer closer(ctx, out)

	var rc io.ReadCloser
	if rc, err = file.Open(); err != nil {
		return err
	}
	defer closer(ctx, rc)

	// Write the body to file
	if _, err = io.CopyN(out, rc, size); err != nil {
		return err
	}

	// Restore original permissions
	return root.Chmod(file.Name, file.Mode())
}

func closer(ctx context.Context, c io.Closer) {
	if err := c.Close(); err != nil {
		slog.ErrorContext(ctx, "close resource", "error", err)
	}
}
