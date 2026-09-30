package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"3.0.0", "3.1.0", -1},
		{"3.1.0", "3.0.0", 1},
		{"3.1.0", "3.1.0", 0},
		{"v3.1.0", "3.1.0", 0}, // a leading v is not a difference
		{"3.0.9", "3.1.0", -1},
		{"2.9.9", "3.0.0", -1},
		{"3.1.2", "3.1.10", -1}, // numeric, not lexical
		{"10.0.0", "9.0.0", 1},
		{"3.1.0-rc1", "3.1.0", 0}, // pre-release suffixes are ignored
		// Unparseable input compares equal so a dev build is never treated
		// as "older" and silently overwritten.
		{"dev", "3.1.0", 0},
		{"3.1.0", "dev", 0},
		{"", "3.1.0", 0},
		{"3.1", "3.1.0", 0},
	}
	for _, tc := range cases {
		if got := Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// The tag carries a leading v and the archive name does not. Conflating
// them is what made the documented download command 404.
func TestAssetName(t *testing.T) {
	cases := []struct{ version, goos, goarch, want string }{
		{"3.1.0", "linux", "amd64", "gitcollect_3.1.0_linux_amd64.tar.gz"},
		{"3.1.0", "darwin", "arm64", "gitcollect_3.1.0_darwin_arm64.tar.gz"},
		{"3.1.0", "windows", "amd64", "gitcollect_3.1.0_windows_amd64.zip"},
	}
	for _, tc := range cases {
		if got := AssetName(tc.version, tc.goos, tc.goarch); got != tc.want {
			t.Errorf("AssetName(%q,%q,%q) = %q, want %q", tc.version, tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	manifest := `a8dc5d6516317e8b5164946097fa9ac7203df3effe0a3d421192d589e0546b13  gitcollect_3.0.0_linux_amd64.tar.gz
f9de4991e770fd6b79e59425e1a63302a5cab70a9ded315e959872614adf10e4  gitcollect_3.0.0_darwin_amd64.tar.gz
`
	got, ok := ChecksumFor(manifest, "gitcollect_3.0.0_linux_amd64.tar.gz")
	if !ok || got != "a8dc5d6516317e8b5164946097fa9ac7203df3effe0a3d421192d589e0546b13" {
		t.Errorf("ChecksumFor = %q, %v", got, ok)
	}
	if _, ok := ChecksumFor(manifest, "gitcollect_3.0.0_windows_arm64.zip"); ok {
		t.Error("expected a missing asset to report not-found")
	}
}

// --- archive helpers ---

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// A README alongside the binary, as the real archives have.
	for _, f := range []struct {
		n string
		b []byte
	}{{"README.md", []byte("readme")}, {name, content}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.n, Mode: 0o755, Size: int64(len(f.b)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinary_TarGz(t *testing.T) {
	want := []byte("\x7fELF fake binary")
	got, err := extractBinary(makeTarGz(t, "gitcollect", want), "linux")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	want := []byte("MZ fake binary")
	got, err := extractBinary(makeZip(t, "gitcollect.exe", want), "windows")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

func TestExtractBinary_MissingBinary(t *testing.T) {
	archive := makeTarGz(t, "something-else", []byte("x"))
	if _, err := extractBinary(archive, "linux"); !errors.Is(err, ErrBinaryNotInArchive) {
		t.Errorf("expected ErrBinaryNotInArchive, got %v", err)
	}
}

// --- end-to-end against a fake release server ---

// releaseServer serves a latest-release document, one platform archive and
// a checksums.txt, so the whole download-and-verify path can run without
// touching the network.
func releaseServer(t *testing.T, version string, archive []byte, corruptChecksum bool) *httptest.Server {
	t.Helper()
	assetName := AssetName(version, runtime.GOOS, runtime.GOARCH)

	sum := sha256.Sum256(archive)
	hexSum := hex.EncodeToString(sum[:])
	if corruptChecksum {
		hexSum = hex.EncodeToString(make([]byte, 32))
	}
	manifest := fmt.Sprintf("%s  %s\n", hexSum, assetName)

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v%s","assets":[
            {"name":%q,"browser_download_url":"%s/dl/asset"},
            {"name":"checksums.txt","browser_download_url":"%s/dl/sums"}]}`,
			version, assetName, srv.URL, srv.URL)
	})
	mux.HandleFunc("/dl/asset", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, manifest) })

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// latestFrom points Latest at a fake release server for the duration of
// one test, so the real network path is exercised end to end without
// reaching GitHub.
func latestFrom(t *testing.T, base string) Release {
	t.Helper()
	saved := releasesURL
	releasesURL = base + "/releases/latest"
	t.Cleanup(func() { releasesURL = saved })

	rel, err := Latest(nil)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	return rel
}

func TestDownload_VerifiesAndExtracts(t *testing.T) {
	want := []byte("fake gitcollect binary")
	name := binaryName(runtime.GOOS)
	var archive []byte
	if runtime.GOOS == "windows" {
		archive = makeZip(t, name, want)
	} else {
		archive = makeTarGz(t, name, want)
	}

	srv := releaseServer(t, "9.9.9", archive, false)
	rel := latestFrom(t, srv.URL)

	if rel.Tag != "v9.9.9" || rel.Version != "9.9.9" {
		t.Fatalf("tag/version = %q/%q, want v9.9.9/9.9.9", rel.Tag, rel.Version)
	}

	got, err := Download(nil, rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("downloaded %q, want %q", got, want)
	}
}

// An archive whose bytes do not match the published checksum must never be
// installed — that verification is the only thing standing between a user
// and an arbitrary binary landing on their PATH.
func TestDownload_RefusesOnChecksumMismatch(t *testing.T) {
	archive := makeTarGz(t, binaryName(runtime.GOOS), []byte("fake"))
	if runtime.GOOS == "windows" {
		archive = makeZip(t, binaryName(runtime.GOOS), []byte("fake"))
	}
	srv := releaseServer(t, "9.9.9", archive, true) // published sum is wrong
	rel := latestFrom(t, srv.URL)

	_, err := Download(nil, rel, runtime.GOOS, runtime.GOARCH)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestDownload_RefusesWhenChecksumAbsent(t *testing.T) {
	archive := makeTarGz(t, binaryName(runtime.GOOS), []byte("fake"))
	srv := releaseServer(t, "9.9.9", archive, false)
	rel := latestFrom(t, srv.URL)
	// Drop the asset from the manifest by renaming what we ask for.
	rel.Assets["gitcollect_9.9.9_plan9_mips.tar.gz"] = rel.Assets[AssetName("9.9.9", runtime.GOOS, runtime.GOARCH)]

	if _, err := Download(nil, rel, "plan9", "mips"); !errors.Is(err, ErrChecksumMissing) {
		t.Errorf("expected ErrChecksumMissing, got %v", err)
	}
}

func TestAssetFor_NoAssetForPlatform(t *testing.T) {
	rel := Release{Version: "3.1.0", Assets: map[string]string{}}
	if _, _, err := rel.AssetFor("linux", "amd64"); !errors.Is(err, ErrNoAsset) {
		t.Errorf("expected ErrNoAsset, got %v", err)
	}
}

// --- binary replacement ---

func TestReplace_SwapsBinaryAndKeepsItExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitcollect")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Replace(path, []byte("new")); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("binary content = %q, want %q", got, "new")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("replacement is not executable: %v", info.Mode())
		}
	}
}

// A failed replacement must leave the original binary in place: the user
// still has a working gitcollect on their PATH.
func TestReplace_LeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitcollect")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new")); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "gitcollect" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}
