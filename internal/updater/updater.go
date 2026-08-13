// Package updater securely replaces Termleaf with a newer GitHub release.
package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

const maximumAssetSize = 100 << 20

// Repository is overridden by release builds when the canonical GitHub owner differs.
var Repository = "tolaniverse/termleaf"

var ErrDevelopmentBuild = errors.New("development builds cannot self-update; install a tagged release first")

type Result struct {
	Previous string
	Current  string
	Updated  bool
}

type release struct {
	TagName    string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Draft      bool    `json:"draft"`
	Assets     []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func Update(ctx context.Context, current string) (Result, error) {
	result := Result{Previous: current, Current: current}
	if current == "dev" || !semver.IsValid(current) {
		return result, ErrDevelopmentBuild
	}

	client := secureHTTPClient()
	latest, err := latestRelease(ctx, client)
	if err != nil {
		return result, err
	}
	if !semver.IsValid(latest.TagName) {
		return result, fmt.Errorf("release has invalid version %q", latest.TagName)
	}
	if semver.Compare(latest.TagName, current) <= 0 {
		return result, nil
	}

	archiveName := fmt.Sprintf("termleaf_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive, ok := findAsset(latest.Assets, archiveName)
	if !ok {
		return result, fmt.Errorf("release %s has no asset for %s/%s", latest.TagName, runtime.GOOS, runtime.GOARCH)
	}
	checksums, ok := findAsset(latest.Assets, "checksums.txt")
	if !ok {
		return result, errors.New("release has no checksums.txt asset")
	}

	checksumData, err := download(ctx, client, checksums)
	if err != nil {
		return result, fmt.Errorf("download checksums: %w", err)
	}
	expected, err := checksumFor(checksumData, archiveName)
	if err != nil {
		return result, err
	}
	archiveData, err := download(ctx, client, archive)
	if err != nil {
		return result, fmt.Errorf("download update: %w", err)
	}
	actual := sha256.Sum256(archiveData)
	if !bytes.Equal(expected, actual[:]) {
		return result, errors.New("update checksum verification failed")
	}
	binary, err := extractBinary(archiveData)
	if err != nil {
		return result, err
	}
	if err := selfupdate.Apply(bytes.NewReader(binary), selfupdate.Options{}); err != nil {
		return result, fmt.Errorf("replace executable: %w", err)
	}
	result.Current = latest.TagName
	result.Updated = true
	return result, nil
}

func secureHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 20 * time.Second
	return &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("refusing insecure update redirect")
		}
		if len(via) >= 10 {
			return errors.New("too many update redirects")
		}
		return nil
	}}
}

func latestRelease(ctx context.Context, client *http.Client) (release, error) {
	url := "https://api.github.com/repos/" + Repository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "termleaf-updater")
	response, err := client.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("check latest release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return release{}, fmt.Errorf("check latest release: GitHub returned %s", response.Status)
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&latest); err != nil {
		return release{}, fmt.Errorf("decode latest release: %w", err)
	}
	if latest.Draft || latest.Prerelease {
		return release{}, errors.New("latest GitHub release is not stable")
	}
	return latest, nil
}

func findAsset(assets []asset, name string) (asset, bool) {
	for _, candidate := range assets {
		if candidate.Name == name {
			return candidate, true
		}
	}
	return asset{}, false
}

func download(ctx context.Context, client *http.Client, item asset) ([]byte, error) {
	parsedURL, err := url.Parse(item.URL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return nil, fmt.Errorf("asset %q has an invalid HTTPS URL", item.Name)
	}
	if item.Size < 0 || item.Size > maximumAssetSize {
		return nil, fmt.Errorf("asset %q is too large", item.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "termleaf-updater")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %q: server returned %s", item.Name, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumAssetSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumAssetSize {
		return nil, fmt.Errorf("asset %q exceeds size limit", item.Name)
	}
	return data, nil
}

func checksumFor(data []byte, filename string) ([]byte, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != filename {
			continue
		}
		checksum, err := hex.DecodeString(fields[0])
		if err != nil || len(checksum) != sha256.Size {
			return nil, fmt.Errorf("invalid checksum for %q", filename)
		}
		return checksum, nil
	}
	return nil, fmt.Errorf("checksums.txt has no entry for %q", filename)
}

func extractBinary(data []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open update archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read update archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || (header.Name != "termleaf" && header.Name != "termleaf.exe") {
			continue
		}
		if header.Size < 1 || header.Size > maximumAssetSize {
			return nil, errors.New("update binary has invalid size")
		}
		binary, err := io.ReadAll(io.LimitReader(tarReader, maximumAssetSize+1))
		if err != nil {
			return nil, fmt.Errorf("read update binary: %w", err)
		}
		return binary, nil
	}
	return nil, errors.New("update archive does not contain termleaf")
}
