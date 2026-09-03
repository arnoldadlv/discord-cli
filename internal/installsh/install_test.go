// Package installsh tests install.sh at the repository root by running it
// against a local fake release server, the way a person would run it
// against GitHub.
package installsh

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func script(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("install.sh missing: %v", err)
	}
	return p
}

// fakeRelease serves binaries and a checksums file under
// /releases/latest/download/ and /releases/download/<tag>/.
func fakeRelease(t *testing.T, badChecksum bool) *httptest.Server {
	t.Helper()
	assets := map[string]string{}
	for _, name := range []string{"discord-darwin-arm64", "discord-darwin-amd64", "discord-linux-amd64", "discord-linux-arm64"} {
		assets[name] = "#!/bin/sh\necho discord version v9.9.9 fake " + name + "\n"
	}
	var sums strings.Builder
	for name, body := range assets {
		sum := sha256.Sum256([]byte(body))
		hexsum := hex.EncodeToString(sum[:])
		if badChecksum {
			hexsum = strings.Repeat("0", 64)
		}
		fmt.Fprintf(&sums, "%s  %s\n", hexsum, name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		base := filepath.Base(r.URL.Path)
		if base == "checksums.txt" {
			_, _ = w.Write([]byte(sums.String()))
			return
		}
		if body, ok := assets[base]; ok && (strings.Contains(r.URL.Path, "/releases/latest/download/") || strings.Contains(r.URL.Path, "/releases/download/v9.9.9/")) {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func run(t *testing.T, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script(t)}, args...)...)
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func TestAssetNameForEachPlatform(t *testing.T) {
	cases := map[[2]string]string{
		{"Darwin", "arm64"}:      "discord-darwin-arm64",
		{"Darwin", "x86_64"}:     "discord-darwin-amd64",
		{"Linux", "x86_64"}:      "discord-linux-amd64",
		{"Linux", "aarch64"}:     "discord-linux-arm64",
		{"Linux", "arm64"}:       "discord-linux-arm64",
		{"MINGW64_NT", "x86_64"}: "",
	}
	for k, want := range cases {
		out, errb, code := run(t, map[string]string{"DISCORD_INSTALL_OS": k[0], "DISCORD_INSTALL_ARCH": k[1]}, "--print-asset")
		if want == "" {
			if code == 0 {
				t.Errorf("%v: expected failure for an unsupported platform, got %q", k, out)
			}
			continue
		}
		if code != 0 || strings.TrimSpace(out) != want {
			t.Errorf("%v: got %q (exit %d, stderr %q), want %q", k, out, code, errb, want)
		}
	}
}

func TestInstallsVerifiedBinary(t *testing.T) {
	srv := fakeRelease(t, false)
	dir := filepath.Join(t.TempDir(), "bin")
	out, errb, code := run(t, map[string]string{
		"DISCORD_RELEASE_BASE": srv.URL + "/releases",
		"DISCORD_INSTALL_DIR":  dir,
		"DISCORD_INSTALL_OS":   "Darwin",
		"DISCORD_INSTALL_ARCH": "arm64",
	})
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errb)
	}
	bin := filepath.Join(dir, "discord")
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("not executable: %v", info.Mode())
	}
	got, _ := exec.Command(bin).Output()
	if !strings.Contains(string(got), "v9.9.9") {
		t.Errorf("installed binary output: %q", got)
	}
	if !strings.Contains(out, "v9.9.9") {
		t.Errorf("script should print the installed version: %q", out)
	}
	if !strings.Contains(out+errb, `export PATH="`+dir) {
		t.Errorf("PATH hint missing when the install dir is not on PATH:\n%s%s", out, errb)
	}
	// A pinned version uses the versioned download path.
	_, errb, code = run(t, map[string]string{
		"DISCORD_RELEASE_BASE": srv.URL + "/releases",
		"DISCORD_INSTALL_DIR":  dir,
		"DISCORD_INSTALL_OS":   "Linux",
		"DISCORD_INSTALL_ARCH": "x86_64",
		"DISCORD_VERSION":      "v9.9.9",
	})
	if code != 0 {
		t.Errorf("pinned version: exit %d: %s", code, errb)
	}
	_, errb, code = run(t, map[string]string{
		"DISCORD_RELEASE_BASE": srv.URL + "/releases",
		"DISCORD_INSTALL_DIR":  dir,
		"DISCORD_VERSION":      "v0.0.0",
	})
	if code == 0 {
		t.Errorf("unknown version should fail: %s", errb)
	}
}

func TestBadChecksumInstallsNothing(t *testing.T) {
	srv := fakeRelease(t, true)
	dir := filepath.Join(t.TempDir(), "bin")
	_, errb, code := run(t, map[string]string{
		"DISCORD_RELEASE_BASE": srv.URL + "/releases",
		"DISCORD_INSTALL_DIR":  dir,
		"DISCORD_INSTALL_OS":   "Linux",
		"DISCORD_INSTALL_ARCH": "aarch64",
	})
	if code == 0 {
		t.Fatalf("expected failure on a checksum mismatch")
	}
	if !strings.Contains(strings.ToLower(errb), "checksum") {
		t.Errorf("stderr should explain: %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "discord")); err == nil {
		t.Errorf("binary installed despite a bad checksum")
	}
}

func TestNoPathHintWhenOnPath(t *testing.T) {
	srv := fakeRelease(t, false)
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, errb, code := run(t, map[string]string{
		"DISCORD_RELEASE_BASE": srv.URL + "/releases",
		"DISCORD_INSTALL_DIR":  dir,
		"DISCORD_INSTALL_OS":   "Darwin",
		"DISCORD_INSTALL_ARCH": "arm64",
		"PATH":                 dir + ":/usr/bin:/bin",
	})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if strings.Contains(out+errb, "export PATH=") {
		t.Errorf("no PATH hint expected when the dir is on PATH:\n%s%s", out, errb)
	}
}
