// Package updater lets claude-teleport upgrade itself in place: it asks GitHub
// for the latest release, downloads the binary built for this machine, verifies
// its SHA-256 checksum, and atomically swaps it for the running executable.
//
// It uses only the standard library, so the tool stays easy to build, and it
// reuses the same release asset names and SHA256SUMS.txt that the install
// script relies on.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// DefaultRepo is the GitHub repository releases are pulled from.
const DefaultRepo = "gowtham-sai-yadav/claude-teleport"

func client() *http.Client { return &http.Client{Timeout: 60 * time.Second} }

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// GitHub rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "claude-teleport-updater")
	return client().Do(req)
}

// LatestVersion returns the tag of the newest published release, e.g. "v0.4.0".
func LatestVersion(ctx context.Context, repo string) (string, error) {
	if repo == "" {
		repo = DefaultRepo
	}
	resp, err := get(ctx, "https://api.github.com/repos/"+repo+"/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("no tag_name in the latest release")
	}
	return rel.TagName, nil
}

// Newer reports whether the release tag `latest` is a higher version than the
// installed version `current`. Both may carry a leading "v".
func Newer(latest, current string) bool {
	return compare(latest, current) > 0
}

// compare returns -1, 0, or 1 comparing two versions by their numeric
// major.minor.patch, ignoring any leading "v" and any pre-release/build suffix.
func compare(a, b string) int {
	x, y := parse(a), parse(b)
	n := len(x)
	if len(y) > n {
		n = len(y)
	}
	for i := 0; i < n; i++ {
		var xi, yi int
		if i < len(x) {
			xi = x[i]
		}
		if i < len(y) {
			yi = y[i]
		}
		if xi != yi {
			if xi < yi {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parse(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// AssetFor returns the release asset name for a given platform. Windows on
// arm64 uses the amd64 build, which runs under emulation, since that is the
// only Windows binary published.
func AssetFor(goos, goarch string) (string, error) {
	if goos == "windows" {
		return "claude-teleport-windows-amd64.exe", nil
	}
	if goos != "linux" && goos != "darwin" {
		return "", fmt.Errorf("no prebuilt binary for %s", goos)
	}
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("no prebuilt binary for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("claude-teleport-%s-%s", goos, goarch), nil
}

// Apply downloads the release binary for this machine, verifies its checksum,
// and replaces the running executable. progress may be nil.
func Apply(ctx context.Context, repo, tag string, progress func(done, total int64)) error {
	if repo == "" {
		repo = DefaultRepo
	}
	asset, err := AssetFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	base := "https://github.com/" + repo + "/releases/download/" + tag

	bin, err := download(ctx, base+"/"+asset, progress)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}

	if err := verifyChecksum(ctx, base+"/SHA256SUMS.txt", asset, bin); err != nil {
		return err
	}

	return replaceExecutable(bin)
}

func download(ctx context.Context, url string, progress func(done, total int64)) ([]byte, error) {
	resp, err := get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %s", resp.Status)
	}
	total := resp.ContentLength
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 32*1024)
	var done int64
	for {
		n, rerr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			return buf, nil
		}
		if rerr != nil {
			return nil, rerr
		}
	}
}

func verifyChecksum(ctx context.Context, sumsURL, asset string, data []byte) error {
	resp, err := get(ctx, sumsURL)
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch checksums: github returned %s", resp.Status)
	}
	sums, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == asset {
			want = f[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum listed for %s", asset)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s (expected %s, got %s)", asset, want, got)
	}
	return nil
}

// replaceExecutable swaps the running program for newContent. On Unix a single
// rename atomically replaces the file even while it is running. On Windows a
// running .exe cannot be overwritten, so the current one is moved aside first.
func replaceExecutable(newContent []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)

	// Write the new binary next to the current one so the rename stays on the
	// same filesystem (and so a permission problem surfaces here, clearly).
	tmp, err := os.CreateTemp(dir, ".claude-teleport-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s. Re-run with sudo, or reinstall with the install script: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(newContent); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return err
	}

	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("cannot replace %s: %w", exe, err)
		}
		if err := os.Rename(tmpName, exe); err != nil {
			os.Rename(old, exe) // roll back
			os.Remove(tmpName)
			return fmt.Errorf("cannot replace %s: %w", exe, err)
		}
		os.Remove(old) // best effort; ignored if the old copy is still running
		return nil
	}

	if err := os.Rename(tmpName, exe); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cannot replace %s. Re-run with sudo, or reinstall with the install script: %w", exe, err)
	}
	return nil
}
