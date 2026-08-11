package integration

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
	"strings"
	"time"
)

// httpClient is shared by every download. The timeout is generous because a
// tool binary is tens of megabytes on a slow connection, but it is finite: a
// CLI that hangs forever on a dead mirror is worse than one that fails.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// latestTag resolves the newest release tag of a GitHub repository.
//
// The CLI resolves "latest" exactly once, at install time, and writes the
// concrete version into react.json. Nothing later re-resolves it. That is the
// difference between a project that builds the same way on every machine and
// one that quietly upgrades its toolchain under someone else's feet.
func latestTag(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolving the latest %s release: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Rate limiting is the common failure here and deserves to be named,
		// because the fix — pass --version — is not obvious from a bare 403.
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return "", fmt.Errorf("GitHub rate-limited the request to resolve the latest %s "+
				"release; pass --version to skip resolution", repo)
		}
		return "", fmt.Errorf("resolving the latest %s release: unexpected status %s", repo, resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding the %s release response: %w", repo, err)
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("the latest %s release has no tag name", repo)
	}
	return payload.TagName, nil
}

// checksums fetches a release's sha256sums.txt and returns it as a map from
// asset name to hex digest. A missing file is reported as a nil map with no
// error, so the caller decides whether to proceed unverified.
func checksums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching checksums: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	sums := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		// The conventional format is "<digest>  <name>", where the name may
		// carry a leading "*" marking a binary-mode file.
		sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	return sums, nil
}

// downloadBinary fetches url into dest, verifies it against wantSHA256 when one
// is supplied, and marks it executable.
//
// The download lands in a temporary file beside the destination and is renamed
// into place only after the digest matches. A half-written or tampered binary
// therefore never appears at the path the build will later execute — which
// matters more here than in most download helpers, because the artifact is a
// program the CLI runs.
func downloadBinary(ctx context.Context, url, dest, wantSHA256 string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, digest), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if wantSHA256 != "" {
		got := hex.EncodeToString(digest.Sum(nil))
		if !strings.EqualFold(got, wantSHA256) {
			return fmt.Errorf("checksum mismatch for %s:\n  expected %s\n  received %s\n"+
				"the download was discarded", url, wantSHA256, got)
		}
	}

	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}
