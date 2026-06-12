package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// repoSlug is the single source of the repo identity for self-upgrade;
// install.sh's REPO variable must stay in sync with it.
const repoSlug = "DhilipBinny/pd-power-monitor"

const (
	releaseLatestAPI = "https://api.github.com/repos/" + repoSlug + "/releases/latest"
	releaseBaseURL   = "https://github.com/" + repoSlug + "/releases/download"
	httpTimeout      = 60 * time.Second
	httpMaxBody      = 50 << 20 // bounds the download; binaries are ~6 MB
)

// Set at release build time via -ldflags "-X main.version=vX.Y.Z"
var version = "dev"

// versionString falls back to Go module/VCS build info so source builds
// report something meaningful instead of "dev".
func versionString() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "dev-" + s.Value[:7]
			}
		}
	}
	return version
}

// semverParts parses "vMAJ.MIN.PATCH"; ok is false for anything else.
func semverParts(v string) (parts [3]int, ok bool) {
	v = strings.TrimPrefix(v, "v")
	fields := strings.SplitN(v, ".", 3)
	if len(fields) != 3 {
		return parts, false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

func semverLess(a, b string) bool {
	pa, oka := semverParts(a)
	pb, okb := semverParts(b)
	if !oka || !okb {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func cmdUpgrade(args []string) {
	checkOnly := false
	target := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			checkOnly = true
		case "--to":
			i++
			if i >= len(args) {
				fmt.Println("--to requires a version tag (e.g. --to v1.2.0)")
				os.Exit(1)
			}
			target = args[i]
		default:
			fmt.Printf("unknown upgrade flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	if target == "" {
		latest, err := fetchLatestTag()
		if err != nil {
			fmt.Printf("failed to fetch latest release: %v\n", err)
			os.Exit(1)
		}
		target = latest
	}

	current := versionString()
	fmt.Printf("Current version: %s\n", current)
	fmt.Printf("Target version:  %s\n", target)

	if current == target {
		fmt.Println("Already up to date.")
		return
	}
	if semverLess(target, current) {
		fmt.Printf("warning: %s is older than the current version — this is a downgrade\n", target)
	}

	if checkOnly {
		fmt.Printf("\nUpgrade available: %s -> %s\n", current, target)
		fmt.Println("Run 'power-monitor upgrade' to install (sudo if installed in a system path).")
		return
	}

	// Be explicit about which copy gets replaced: this is os.Executable(),
	// not necessarily the installed /usr/local/bin binary.
	exePath, err := currentExecutable()
	if err != nil {
		fmt.Printf("cannot resolve current binary path: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("This will replace: %s\n", exePath)

	asset := fmt.Sprintf("power-monitor-%s-%s", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("\nDownloading %s %s...\n", asset, target)

	sums, err := httpGet(releaseBaseURL + "/" + target + "/SHA256SUMS")
	if err != nil {
		fmt.Printf("failed to fetch SHA256SUMS: %v\n", err)
		os.Exit(1)
	}
	expected := findChecksum(string(sums), asset)
	if expected == "" {
		fmt.Printf("no checksum for %s in SHA256SUMS of %s\n", asset, target)
		os.Exit(1)
	}

	binData, err := httpGet(releaseBaseURL + "/" + target + "/" + asset)
	if err != nil {
		fmt.Printf("download failed: %v\n", err)
		os.Exit(1)
	}
	if int64(len(binData)) >= httpMaxBody {
		fmt.Println("release asset exceeds the size limit — aborting")
		os.Exit(1)
	}
	sum := sha256.Sum256(binData)
	if hex.EncodeToString(sum[:]) != expected {
		fmt.Println("checksum mismatch — aborting, binary not replaced")
		os.Exit(1)
	}
	fmt.Printf("Verified %d bytes (sha256 OK)\n", len(binData))

	if err := replaceBinary(exePath, binData); err != nil {
		fmt.Printf("install failed: %v\n", err)
		if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
			fmt.Println("the binary lives in a system path — retry with: sudo power-monitor upgrade")
		}
		os.Exit(1)
	}

	fmt.Printf("\nUpgraded %s -> %s\n", current, target)
	// readPID can't see the user's indicator when running under sudo, so
	// the hint is unconditional
	fmt.Println("If the indicator is running, restart it with: power-monitor restart")
}

func currentExecutable() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	return exePath, nil
}

func fetchLatestTag() (string, error) {
	body, err := httpGet(releaseLatestAPI)
	if err != nil {
		return "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("parse release: %w", err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf("no release tag found")
	}
	return release.TagName, nil
}

// findChecksum extracts the hex sha256 for asset from a SHA256SUMS body.
// Line format: "<hex>  <filename>".
func findChecksum(sums, asset string) string {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			return fields[0]
		}
	}
	return ""
}

// replaceBinary writes binData next to exePath and atomically renames it
// into place. POSIX rename-over-open-file keeps any running process on its
// old inode, so an active indicator keeps working until restarted.
func replaceBinary(exePath string, binData []byte) error {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, "power-monitor-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(binData); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "power-monitor-upgrade/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, httpMaxBody))
}
