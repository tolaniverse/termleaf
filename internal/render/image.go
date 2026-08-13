package render

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	termimg "github.com/blacktop/go-termimg"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxImageHeight = 12
	maxImageBytes  = 25 << 20
	maxImagePixels = 40_000_000
	maxImageSide   = 32_768
)

// Image renders an absolute local image as ANSI Unicode half-blocks. Half-blocks
// remain in Bubble Tea's text buffer, so they scroll, resize, and diff cleanly.
func (c *Cache) Image(destination, alt string, width int) string {
	if c.imageMode == ImageOff || c.protocol == ProtocolOff {
		return imageDisabledPlaceholder(alt, destination, width)
	}
	path, info, err := inspectLocalImage(destination)
	if err != nil {
		return imagePlaceholder(alt, destination, err, width)
	}

	key := fmt.Sprintf("image:%s:%d:%s:%d:%d", c.protocol, width, path, info.Size(), info.ModTime().UnixNano())
	if value, ok := c.get(key); ok {
		return value
	}

	output, err := renderImageAtHeight(path, width, maxImageHeight, c.protocol)
	if err != nil {
		return imagePlaceholder(alt, destination, err, width)
	}
	output = strings.TrimRight(output, "\n")
	c.put(key, output)
	return output
}

func renderImageAtHeight(path string, width, height int, protocol ImageProtocol) (string, error) {
	terminalImage, err := termimg.Open(path)
	if err != nil {
		return "", err
	}
	terminalImage = terminalImage.
		Width(max(1, width)).
		Height(max(1, height)).
		Scale(termimg.ScaleFit).
		Protocol(termimgProtocol(protocol))
	if protocol == ProtocolKitty {
		terminalImage = terminalImage.UseUnicode(true).PNG(true).Compression(true)
	}
	return terminalImage.Render()
}

func inspectLocalImage(destination string) (path string, info os.FileInfo, err error) {
	parsed, err := url.Parse(destination)
	if err != nil {
		return "", nil, fmt.Errorf("invalid image path")
	}
	if parsed.Scheme != "" && parsed.Scheme != "file" {
		return "", nil, fmt.Errorf("remote images are disabled")
	}
	if parsed.Scheme == "file" && parsed.Host != "" && parsed.Host != "localhost" {
		return "", nil, fmt.Errorf("remote file URLs are disabled")
	}
	if parsed.Path == "" {
		return "", nil, fmt.Errorf("image path is empty")
	}
	if !filepath.IsAbs(parsed.Path) {
		return "", nil, fmt.Errorf("relative image path was not resolved")
	}

	path = filepath.Clean(parsed.Path)
	info, err = os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("inspect image: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("image is not a regular file")
	}
	if info.Size() > maxImageBytes {
		return "", nil, fmt.Errorf("image exceeds %d MiB limit", maxImageBytes>>20)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("open image: %w", err)
	}
	config, _, decodeErr := image.DecodeConfig(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return "", nil, fmt.Errorf("inspect image dimensions: %w", decodeErr)
	}
	if closeErr != nil {
		return "", nil, fmt.Errorf("close image: %w", closeErr)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageSide || config.Height > maxImageSide || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return "", nil, fmt.Errorf("image dimensions exceed safety limit")
	}
	return path, info, nil
}

func imageDisabledPlaceholder(alt, destination string, width int) string {
	label := strings.TrimSpace(sanitizeTerminalText(alt))
	if label == "" {
		label = sanitizeTerminalText(filepath.Base(destination))
	}
	return ansi.Truncate("[image: "+label+"]", max(1, width), "…")
}

func imagePlaceholder(alt, destination string, cause error, width int) string {
	label := strings.TrimSpace(sanitizeTerminalText(alt))
	if label == "" {
		label = sanitizeTerminalText(filepath.Base(destination))
	}
	message := fmt.Sprintf("[image: %s — %s]", label, sanitizeTerminalText(cause.Error()))
	return ansi.Truncate(message, max(1, width), "…")
}

func sanitizeTerminalText(value string) string {
	value = ansi.Strip(value)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}
