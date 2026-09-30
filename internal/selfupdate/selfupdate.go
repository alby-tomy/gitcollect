// Package selfupdate finds and installs newer gitcollect releases.
//
// It deals with gitcollect's own distribution, not with a user's repos, so
// it deliberately does not reuse internal/api: that package speaks the
// GitHub collaborator API as an authenticated user, whereas everything
// here is unauthenticated reads of one public repository's releases.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// Module is the import path "go install" needs. The /v3 suffix is not
	// optional: without it Go resolves the module's v1 line and installs
	// v1.0.0 while exiting successfully.
	Module = "github.com/alby-tomy/gitcollect/v3"

	// checksumsAsset is goreleaser's checksum manifest, one
	// "<sha256>  <filename>" line per archive.
	checksumsAsset = "checksums.txt"

	requestTimeout = 30 * time.Second
	// maxArchiveBytes bounds what will be read out of a downloaded archive,
	// so a malformed or hostile entry cannot exhaust memory or disk.
	maxArchiveBytes = 128 << 20 // 128 MiB
)

// releasesURL is the unauthenticated endpoint for the newest release. It
// is a var, not a const, so selfupdate_test.go can point it at an
// httptest.Server — the same reason internal/api keeps githubBaseURL one.
// Production code never reassigns it.
var releasesURL = "https://api.github.com/repos/alby-tomy/gitcollect/releases/latest"

var (
	// ErrNoAsset is returned when the latest release has no archive built
	// for the running platform.
	ErrNoAsset = errors.New("no release asset for this platform")
	// ErrChecksumMissing is returned when checksums.txt does not list the
	// asset being installed — the download is then unverifiable and is
	// refused rather than trusted.
	ErrChecksumMissing = errors.New("asset not listed in checksums.txt")
	// ErrChecksumMismatch is returned when the downloaded bytes do not
	// match the published checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")
	// ErrBinaryNotInArchive is returned when the archive contains no
	// gitcollect executable.
	ErrBinaryNotInArchive = errors.New("archive contains no gitcollect binary")
)

// Release is the subset of a GitHub release gitcollect needs to upgrade
// itself.
type Release struct {
	// Tag is the git tag, e.g. "v3.1.0".
	Tag string
	// Version is Tag without the leading "v", e.g. "3.1.0". goreleaser
	// strips it from archive names, so the two are not interchangeable —
	// conflating them builds a download URL that 404s.
	Version string
	// Assets maps asset filename to its download URL.
	Assets map[string]string
}

// Latest fetches the newest published release.
func Latest(client *http.Client) (Release, error) {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}

	req, err := http.NewRequest(http.MethodGet, releasesURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub returned %s when looking up the latest release", resp.Status)
	}

	var out struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Release{}, fmt.Errorf("could not parse the release response: %w", err)
	}
	if out.TagName == "" {
		return Release{}, errors.New("the latest release has no tag name")
	}

	rel := Release{
		Tag:     out.TagName,
		Version: strings.TrimPrefix(out.TagName, "v"),
		Assets:  make(map[string]string, len(out.Assets)),
	}
	for _, a := range out.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// AssetName returns the archive goreleaser publishes for goos/goarch at
// version. version must be the bare number ("3.1.0"), not the tag.
func AssetName(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("gitcollect_%s_%s_%s.%s", version, goos, goarch, ext)
}

// Compare orders two version strings, ignoring any leading "v": it returns
// -1 when a precedes b, +1 when a follows b, and 0 when they are equal or
// either is not a plain numeric version.
//
// Returning 0 for unparseable input is deliberate. A development build
// reports "dev", and treating that as "older" would prompt developers to
// overwrite their own build with a release; callers check for those cases
// explicitly instead.
func Compare(a, b string) int {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

// parseVersion splits "v3.1.0" or "3.1.0" into its numeric components. Any
// pre-release or build suffix is ignored, so "3.1.0-rc1" compares equal to
// "3.1.0" — gitcollect does not publish pre-releases, and guessing at their
// ordering would be worse than declining to.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i != -1 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AssetFor returns the download URL of the archive for the running
// platform, or ErrNoAsset when the release has none.
func (r Release) AssetFor(goos, goarch string) (name, url string, err error) {
	name = AssetName(r.Version, goos, goarch)
	url, ok := r.Assets[name]
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrNoAsset, name)
	}
	return name, url, nil
}

// Download fetches the release archive for the running platform, verifies
// it against the published checksums, and returns the extracted gitcollect
// binary as a byte slice.
//
// Verification is not optional: an asset missing from checksums.txt is
// refused rather than installed unverified, because the whole point of
// replacing a binary on someone's PATH is that they trust what lands there.
func Download(client *http.Client, rel Release, goos, goarch string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}

	name, url, err := rel.AssetFor(goos, goarch)
	if err != nil {
		return nil, err
	}

	archive, err := fetch(client, url)
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", name, err)
	}

	sums, err := fetch(client, rel.Assets[checksumsAsset])
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", checksumsAsset, err)
	}
	want, ok := ChecksumFor(string(sums), name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChecksumMissing, name)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%w for %s: expected %s, got %s",
			ErrChecksumMismatch, name, want, hex.EncodeToString(got[:]))
	}

	return extractBinary(archive, goos)
}

func fetch(client *http.Client, url string) ([]byte, error) {
	if url == "" {
		return nil, errors.New("no download URL")
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes))
}

// ChecksumFor finds the sha256 recorded for name in a goreleaser
// checksums.txt, whose lines are "<hex sha256>  <filename>".
func ChecksumFor(manifest, name string) (string, bool) {
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[1] == name {
			return fields[0], true
		}
	}
	return "", false
}

// binaryName is the executable's name inside a release archive.
func binaryName(goos string) string {
	if goos == "windows" {
		return "gitcollect.exe"
	}
	return "gitcollect"
}

// extractBinary pulls the gitcollect executable out of a release archive.
func extractBinary(archive []byte, goos string) ([]byte, error) {
	if goos == "windows" {
		return extractFromZip(archive, binaryName(goos))
	}
	return extractFromTarGz(archive, binaryName(goos))
}

func extractFromTarGz(archive []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("could not read archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("could not read archive: %w", err)
		}
		// Match on the base name: the archive is flat today, but a future
		// layout change should not silently stop finding the binary.
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == want {
			return io.ReadAll(io.LimitReader(tr, maxArchiveBytes))
		}
	}
	return nil, fmt.Errorf("%w (looking for %s)", ErrBinaryNotInArchive, want)
}

func extractFromZip(archive []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("could not read archive: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("could not read %s: %w", f.Name, err)
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, maxArchiveBytes))
	}
	return nil, fmt.Errorf("%w (looking for %s)", ErrBinaryNotInArchive, want)
}

// Replace installs binary at path, preserving the original if anything
// fails.
//
// The new file is written alongside the target and renamed into place, so
// the swap is atomic and a partial download can never be left executable.
// Windows cannot rename over a running executable, so the current one is
// moved aside first and removed afterwards on a best-effort basis; the
// leftover is harmless and the next successful update clears it.
func Replace(path string, binary []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".gitcollect-update-*")
	if err != nil {
		return fmt.Errorf("could not write to %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename succeeds

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write the new binary: %w", err)
	}
	// Flush to disk before the rename. os.Rename makes the directory entry
	// atomic, not the file's contents: without this, a crash between the
	// rename and the kernel's writeback leaves a correctly named file
	// holding zero or partial bytes, which here would leave an unrunnable gitcollect on the PATH.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write the new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not write the new binary: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("could not make the new binary executable: %w", err)
	}

	if runtime.GOOS == "windows" {
		old := path + ".old"
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			return fmt.Errorf("could not move the running binary aside: %w", err)
		}
		if err := os.Rename(tmpPath, path); err != nil {
			// Put the original back rather than leave nothing on PATH.
			_ = os.Rename(old, path)
			return fmt.Errorf("could not install the new binary: %w", err)
		}
		_ = os.Remove(old)
		return nil
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("could not install the new binary: %w", err)
	}
	return nil
}
