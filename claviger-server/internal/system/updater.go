package system // or wherever your cron file lives

import (
	"claviger-server/storage"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Define your current running version
// This should match the VERSION variable in your bash script
const CurrentVersion = "v0.3.0"

// GithubRelease maps the JSON response from GitHub's API
type GithubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// CheckGitHubForUpdates pings the GitHub API and compares versions
func CheckGitHubForUpdates() (bool, string, error) {
	url := "https://api.github.com/repos/Dncky35/claviger/releases/latest"

	// Security: Always use a timeout so a hanging network doesn't freeze the cron worker
	client := http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, "", err
	}

	// GitHub API politely requests a User-Agent
	req.Header.Set("User-Agent", "Claviger-Zero-Trust-Gateway")

	res, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("GitHub API returned status: %d", res.StatusCode)
	}

	var release GithubRelease
	if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
		return false, "", err
	}

	// If the tag on GitHub doesn't match our current version, an update is available!
	// Note: You can implement more complex semver comparison later if needed.
	db := storage.InitDB()
	defer db.Close()

	currentVersion := storage.GetConfig(db, "current_version")
	if currentVersion == "" {
		currentVersion = CurrentVersion
	}

	if release.TagName != currentVersion {
		return true, release.TagName, nil
	}

	return false, release.TagName, nil
}
