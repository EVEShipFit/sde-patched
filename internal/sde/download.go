package sde

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var baseURL = "https://sde.evetech.net"

var client = &http.Client{Timeout: 30 * time.Minute}

type release struct {
	BuildNumber int32 `json:"buildNumber"`
	Tables      map[string]struct {
		JSONL struct {
			URL string `json:"url"`
		} `json:"jsonl"`
	} `json:"tables"`
}

// LatestBuild returns the build number of the SDE currently on the given server.
func LatestBuild(server string) (int32, error) {
	var latest release
	if err := getJSON(fmt.Sprintf("%s/%s/latest/", baseURL, server), &latest); err != nil {
		return 0, err
	}
	if latest.BuildNumber == 0 {
		return 0, fmt.Errorf("%s: no latest build", server)
	}
	return latest.BuildNumber, nil
}

// Download fetches the tables of a build into dir/<build>, unless already there, and returns that path.
func Download(dir string, server string, build int32) (string, error) {
	target := filepath.Join(dir, strconv.Itoa(int(build)))

	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	found, err := findRelease(server, build)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// Download into a temporary directory first, so an interrupted download is
	// never mistaken for a complete one.
	tmp, err := os.MkdirTemp(dir, ".download-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	for _, table := range tables {
		url := found.Tables[table].JSONL.URL
		if url == "" {
			return "", fmt.Errorf("build %d has no table %s", build, table)
		}
		if err := fetch(url, filepath.Join(tmp, table+".jsonl.gz")); err != nil {
			return "", err
		}
	}

	// MkdirTemp makes the directory private; the SDE is not a secret.
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return target, nil
		}
		return "", err
	}
	return target, nil
}

// LocalBuild returns the build number of the newest SDE in dir, or zero if
// there is none.
func LocalBuild(dir string) int32 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	var newest int32
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		build, err := strconv.ParseInt(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		if int32(build) > newest {
			newest = int32(build)
		}
	}
	return newest
}

// findRelease returns the newest release of the given build.
func findRelease(server string, build int32) (*release, error) {
	var index struct {
		Releases []struct {
			ReleaseURL string `json:"releaseUrl"`
		} `json:"releases"`
	}
	if err := getJSON(fmt.Sprintf("%s/%s/", baseURL, server), &index); err != nil {
		return nil, err
	}

	for _, entry := range index.Releases {
		var candidate release
		if err := getJSON(entry.ReleaseURL, &candidate); err != nil {
			return nil, err
		}
		if candidate.BuildNumber == build {
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("no release of build %d on %s", build, server)
}

func getJSON(url string, v any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

// fetch stores the body of url in filename as it is sent: gzip-compressed.
func fetch(url string, filename string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
		return fmt.Errorf("%s: expected gzip, got %q", url, encoding)
	}

	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close()
		return fmt.Errorf("%s: %w", url, err)
	}
	return file.Close()
}
