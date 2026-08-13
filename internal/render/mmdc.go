package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	mmdcTimeout       = 8 * time.Second
	maxMMDCPNGBytes   = 20 << 20
	maxMMDCLogBytes   = 64 << 10
	maxMMDCRenderRows = 18
)

// MMDCRenderer optionally renders Mermaid with an installed mmdc binary.
type MMDCRenderer struct {
	path string
}

func NewMMDCRenderer(path string) *MMDCRenderer {
	if path == "" {
		return nil
	}
	return &MMDCRenderer{path: path}
}

func FindMMDC() *MMDCRenderer {
	path, err := exec.LookPath("mmdc")
	if err != nil {
		return nil
	}
	return NewMMDCRenderer(path)
}

func (r *MMDCRenderer) identity() string {
	if r == nil {
		return "embedded"
	}
	return "mmdc:" + r.path
}

// Render generates a bounded PNG in an isolated temporary directory.
func (r *MMDCRenderer) Render(ctx context.Context, source string, width int, cache *Cache) (string, error) {
	if r == nil || r.path == "" {
		return "", fmt.Errorf("mmdc is not installed")
	}
	if cache == nil {
		return "", fmt.Errorf("mmdc render cache is nil")
	}
	if !cache.MMDCEnabled() {
		return "", fmt.Errorf("mmdc rendering is not enabled")
	}
	if cache.protocol == ProtocolOff {
		return "", fmt.Errorf("images are disabled")
	}
	if err := validateMermaidInput(source); err != nil {
		return "", err
	}

	digest := sha256.Sum256([]byte(source))
	key := fmt.Sprintf("mmdc:%s:%d:%x", r.path, width, digest)
	if value, ok := cache.get(key); ok {
		return value, nil
	}

	temporaryDirectory, err := os.MkdirTemp("", "termleaf-mermaid-*")
	if err != nil {
		return "", fmt.Errorf("create Mermaid temporary directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	inputPath := filepath.Join(temporaryDirectory, "diagram.mmd")
	outputPath := filepath.Join(temporaryDirectory, "diagram.png")
	if err := os.WriteFile(inputPath, []byte(source), 0o600); err != nil {
		return "", fmt.Errorf("write Mermaid source: %w", err)
	}

	timedContext, cancel := context.WithTimeout(ctx, mmdcTimeout)
	defer cancel()
	command := exec.Command(r.path,
		"--input", inputPath,
		"--output", outputPath,
		"--outputFormat", "png",
		"--quiet",
	)
	command.Dir = temporaryDirectory
	command.Env = safeMMDCEnvironment()
	isolateProcess(command)

	var diagnostics cappedBuffer
	command.Stdout = &diagnostics
	command.Stderr = &diagnostics
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("start mmdc: %w", err)
	}

	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-waitResult:
	case <-timedContext.Done():
		_ = killProcessTree(command)
		<-waitResult
		return "", fmt.Errorf("mmdc exceeded %s timeout", mmdcTimeout)
	}
	if waitErr != nil {
		return "", fmt.Errorf("mmdc failed: %s", sanitizeTerminalText(diagnostics.String()))
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return "", fmt.Errorf("inspect mmdc output: %w", err)
	}
	if info.Size() > maxMMDCPNGBytes {
		return "", fmt.Errorf("mmdc output exceeds %d MiB limit", maxMMDCPNGBytes>>20)
	}
	if _, _, err := inspectLocalImage(outputPath); err != nil {
		return "", fmt.Errorf("validate mmdc output: %w", err)
	}

	terminalImage, err := renderImageAtHeight(outputPath, width, maxMMDCRenderRows, cache.protocol)
	if err != nil {
		return "", fmt.Errorf("render mmdc output: %w", err)
	}
	cache.put(key, terminalImage)
	return terminalImage, nil
}

func safeMMDCEnvironment() []string {
	keys := []string{
		"HOME", "PATH", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL",
		"NODE_PATH", "PUPPETEER_EXECUTABLE_PATH", "SystemRoot", "USERPROFILE",
		"LOCALAPPDATA", "APPDATA",
	}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

type cappedBuffer struct {
	buffer bytes.Buffer
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	remaining := maxMMDCLogBytes - b.buffer.Len()
	if remaining > 0 {
		_, _ = b.buffer.Write(value[:min(remaining, len(value))])
	}
	return len(value), nil
}

func (b *cappedBuffer) String() string {
	value, _ := io.ReadAll(bytes.NewReader(b.buffer.Bytes()))
	return string(value)
}
