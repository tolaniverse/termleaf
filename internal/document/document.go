package document

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// Document is a local Markdown document and its stable storage identity.
type Document struct {
	ID       string
	Path     string
	Name     string
	Markdown string
}

// Open reads a local Markdown file.
func Open(path string) (Document, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return Document{}, fmt.Errorf("resolve document path: %w", err)
	}

	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return Document{}, fmt.Errorf("resolve document symlinks: %w", err)
	}

	contents, err := os.ReadFile(canonicalPath)
	if err != nil {
		return Document{}, fmt.Errorf("read document: %w", err)
	}

	identity := sha256.Sum256(contents)
	return Document{
		ID:       fmt.Sprintf("%x", identity),
		Path:     canonicalPath,
		Name:     filepath.Base(canonicalPath),
		Markdown: string(contents),
	}, nil
}
