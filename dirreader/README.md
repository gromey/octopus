# DirReader

## Overview

`dirreader` provides fast, concurrent directory reader with optional file hashing
and include/exclude filtering.  
It is ideal for:

- checksum generation
- file indexing
- folder synchronization tools
- scanning large directories efficiently

---

## ✨ Features

- 🚀 **Concurrent scanning** of directories and files
- 🔍 **Optional hashing** using any `hash.Hash` implementation
- 🎯 **Include/exclude filtering** by file extension

---

## Usage Examples

```go
package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/gromey/octopus/dirreader"
)

func main() {
	files, err := dirreader.Exec(
		"/path/to/root",
		sha256.New,              // optional hash function
		[]string{".go", ".txt"}, // filter mask
		true,                    // include only files with these extensions
	)
	if err != nil {
		panic(err)
	}

	for _, f := range files {
		fmt.Println(f.Name(), f.PathRel, f.Hash)
	}
}

```

---

### DeleteEmptyDirectories(root string)

Removes all empty folders in a directory tree.
