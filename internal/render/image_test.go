package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestImageRendersLocalFileAsTerminalCells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gradient.png")
	fixture := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			fixture.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 120, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	if err := png.Encode(file, fixture); err != nil {
		_ = file.Close()
		t.Fatalf("encode image: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close image: %v", err)
	}

	output := NewCache(1<<20).Image(path, "gradient", 10)
	if strings.Contains(output, "[image:") {
		t.Fatalf("image fell back to placeholder: %q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if width := ansi.StringWidth(line); width > 10 {
			t.Fatalf("image line width = %d, want <= 10", width)
		}
	}
}

func TestImageOffDoesNotReadFile(t *testing.T) {
	output := NewCacheWithImages(1024, ImageOff).Image("/missing/image.png", "architecture", 80)
	if output != "[image: architecture]" {
		t.Fatalf("disabled image = %q", output)
	}
}

func TestImageProtocolChangesCacheIdentity(t *testing.T) {
	cache := NewCacheWithImages(1024, ImagePixels)
	cache.put("image:halfblocks:10:test", "pixels")
	cache.protocol = ProtocolKitty
	if value, ok := cache.get("image:kitty:10:test"); ok {
		t.Fatalf("Kitty cache unexpectedly reused %q", value)
	}
}

func TestImagePlaceholderStripsTerminalControlSequences(t *testing.T) {
	output := NewCache(1024).Image("https://example.com/image.png", "safe\x1b]52;c;payload\a text", 80)
	if strings.ContainsAny(output, "\x1b\a") || !strings.Contains(output, "safe") {
		t.Fatalf("unsafe placeholder = %q", output)
	}
}

func TestImageRejectsRemoteFileAuthority(t *testing.T) {
	output := NewCache(1024).Image("file://remote-host/tmp/image.png", "network file", 80)
	if !strings.Contains(output, "remote file URLs are disabled") {
		t.Fatalf("fallback = %q", output)
	}
}

func TestImageUsesReadableFallback(t *testing.T) {
	output := NewCache(1024).Image("https://example.com/image.png", "remote diagram", 80)
	if !strings.Contains(output, "remote diagram") || !strings.Contains(output, "remote images are disabled") {
		t.Fatalf("fallback = %q", output)
	}
}
