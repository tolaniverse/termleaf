package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestChecksumFor(t *testing.T) {
	payload := []byte("archive")
	hash := sha256.Sum256(payload)
	checksums := []byte(fmt.Sprintf("%x  termleaf_linux_amd64.tar.gz\n", hash))
	got, err := checksumFor(checksums, "termleaf_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("checksumFor: %v", err)
	}
	if !bytes.Equal(got, hash[:]) {
		t.Fatalf("checksum = %x, want %x", got, hash)
	}
}

func TestSecureHTTPClientRejectsHTTPSDowngrade(t *testing.T) {
	client := secureHTTPClient()
	err := client.CheckRedirect(&http.Request{URL: &url.URL{Scheme: "http", Host: "example.com"}}, nil)
	if err == nil {
		t.Fatal("insecure redirect was accepted")
	}
}

func TestDownloadRejectsInsecureURL(t *testing.T) {
	_, err := download(t.Context(), secureHTTPClient(), asset{Name: "termleaf.tar.gz", URL: "http://example.com/termleaf.tar.gz", Size: 1})
	if err == nil {
		t.Fatal("insecure asset URL was accepted")
	}
}

func TestExtractBinary(t *testing.T) {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	payload := []byte("termleaf binary")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "termleaf", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tarWriter.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	got, err := extractBinary(archive.Bytes())
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("binary = %q, want %q", got, payload)
	}
}
