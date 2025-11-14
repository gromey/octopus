# Zipper

## Overview

`zipper` provides simple, flexible helpers for creating and extracting ZIP archives.
It focuses on readability, correctness, and safe filesystem handling.

---

## ✨ Features

- 📁 Zip entire directories
- 🗺️ Zip files using custom mapping
- 📝 Zip arbitrary in-memory data
- 🧠 Create ZIP archives entirely in memory
- 💾 Write ZIPs directly to disk
- 🔐 Secure unzip with max-file-size validation
- 🧱 Uses Go’s `os.OpenRoot` for path-safe operations

---

## Usage Examples

```go
package main

import (
	"context"

	"github.com/gromey/octopus/zipper"
)

func main() {
	ctx := context.Background()

	// Zip a directory to a file
	writer := zipper.ZipSrcDir("/path/to/src", "archive.zip")

	if err := zipper.ZipToFile(ctx, writer, "/output/archive.zip"); err != nil {
		panic(err)
	}

	// Zip specific files with their own paths
	writer = zipper.ZipSrcMap(map[string]string{
		"/path/image.jpg":  "images",
		"/path/report.pdf": "docs",
	})

	if err := zipper.ZipToFile(ctx, writer, "out.zip"); err != nil {
		panic(err)
	}

	// Produces:
	// out.zip
	// ├── images/
	// │   └── image.jpg
	// └── docs/
	//     └── report.pdf

	// Zip in-memory data
	writer = zipper.ZipSrcData("hello.txt", []byte("Hello"))

	buf, err := zipper.ZipInMemory(ctx, writer)
	if err != nil {
		panic(err)
	}
	_ = buf.Bytes()

	// Unzip with security checks
	var limit int64 = 10 << 20 // 10 MB limit

	if err = zipper.Unzip(ctx, "archive.zip", "/dst", limit); err != nil {
		panic(err)
	}
}

```
