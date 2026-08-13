package document

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityFollowsContentAcrossPaths(t *testing.T) {
	directory := t.TempDir()
	firstPath := filepath.Join(directory, "first.md")
	secondPath := filepath.Join(directory, "second.md")
	for _, path := range []string{firstPath, secondPath} {
		if err := os.WriteFile(path, []byte("# Same document\n"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	first, err := Open(firstPath)
	if err != nil {
		t.Fatalf("open first document: %v", err)
	}
	second, err := Open(secondPath)
	if err != nil {
		t.Fatalf("open second document: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("same content IDs differ: %q != %q", first.ID, second.ID)
	}
}
