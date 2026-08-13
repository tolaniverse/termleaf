package render

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMMDCRendererUsesIsolatedFilesAndReturnsTerminalImage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "mmdc")
	pngPath := filepath.Join(directory, "fixture.png")
	writePNGFixture(t, pngPath)
	script := `#!/bin/sh
set -eu
input=""; output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --input) input="$2"; shift 2 ;;
    --output) output="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ -f "$input" ]
cp "` + pngPath + `" "$output"
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake mmdc: %v", err)
	}

	cache := NewCache(1 << 20)
	cache.EnableMMDC(true)
	output, err := NewMMDCRenderer(binary).Render(context.Background(), "graph LR\nA-->B", 20, cache)
	if err != nil {
		t.Fatalf("render with fake mmdc: %v", err)
	}
	if output == "" || strings.Contains(output, "mmdc failed") {
		t.Fatalf("terminal image output = %q", output)
	}
}

func TestMMDCRendererRequiresOptInAndImages(t *testing.T) {
	renderer := NewMMDCRenderer("mmdc")
	if _, err := renderer.Render(context.Background(), "graph LR\nA-->B", 20, NewCache(1024)); err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("disabled mmdc error = %v", err)
	}
	cache := NewCacheWithImages(1024, ImageOff)
	cache.EnableMMDC(true)
	if _, err := renderer.Render(context.Background(), "graph LR\nA-->B", 20, cache); err == nil || !strings.Contains(err.Error(), "images are disabled") {
		t.Fatalf("images-off mmdc error = %v", err)
	}
}

func TestMMDCRendererReportsMissingExecutable(t *testing.T) {
	cache := NewCache(1024)
	cache.EnableMMDC(true)
	_, err := NewMMDCRenderer(filepath.Join(t.TempDir(), "missing")).Render(context.Background(), "graph LR\nA-->B", 20, cache)
	if err == nil || !strings.Contains(err.Error(), "start mmdc") {
		t.Fatalf("error = %v", err)
	}
}

func writePNGFixture(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create PNG fixture: %v", err)
	}
	fixture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			fixture.Set(x, y, color.RGBA{R: 220, G: 80, B: 40, A: 255})
		}
	}
	if err := png.Encode(file, fixture); err != nil {
		_ = file.Close()
		t.Fatalf("encode PNG fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close PNG fixture: %v", err)
	}
}
