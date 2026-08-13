package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKittyRenderUsesGraphicsAndUnicodePlaceholders(t *testing.T) {
	path := protocolFixture(t)
	cache := NewCacheWithImages(1<<20, ImagePixels)
	cache.protocol = ProtocolKitty
	output := cache.Image(path, "fixture", 8)
	if !strings.Contains(output, "\x1b_G") {
		t.Fatalf("Kitty output lacks graphics command: %q", output)
	}
	if !strings.ContainsRune(output, '\U0010eeee') {
		t.Fatal("Kitty output lacks Unicode placeholders")
	}
}

func TestITerm2RenderUsesOSC1337(t *testing.T) {
	path := protocolFixture(t)
	cache := NewCacheWithImages(1<<20, ImagePixels)
	cache.protocol = ProtocolITerm2
	output := cache.Image(path, "fixture", 8)
	if !strings.Contains(output, "]1337;") {
		t.Fatalf("iTerm2 output lacks OSC 1337: %q", output)
	}
}

func TestSixelRenderUsesDCS(t *testing.T) {
	path := protocolFixture(t)
	cache := NewCacheWithImages(1<<20, ImagePixels)
	cache.protocol = ProtocolSixel
	output := cache.Image(path, "fixture", 8)
	if !strings.Contains(output, "\x1bPq") {
		t.Fatalf("Sixel output lacks DCS: %q", output)
	}
}

func protocolFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	fixture := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			fixture.Set(x, y, color.RGBA{R: 200, G: uint8(y * 20), B: uint8(x * 20), A: 255})
		}
	}
	if err := png.Encode(file, fixture); err != nil {
		_ = file.Close()
		t.Fatalf("encode fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return path
}
