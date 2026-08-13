package system

import (
	"claviger-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"time"

	"golang.org/x/mod/semver" // Standard Go package for version comparison
)

// CurrentVersion is the hardcoded fallback version of the compiled binary
const CurrentVersion = "v0.3.15"

// GithubRelease maps the JSON response from GitHub's API
type GithubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
}

// CheckGitHubForUpdates pings the GitHub API and compares versions based on the selected channel
func CheckGitHubForUpdates() (bool, string, error) {
	db := storage.InitDB()
	defer db.Close()

	// 1. Get currently installed version
	currentVersion := storage.GetConfig(db, "installed_version")
	if currentVersion == "" {
		currentVersion = CurrentVersion
	}
	// log.Printf("[UpdateCheck] 🔍 Starting update check. Current installed version: '%s'", currentVersion)
	// log.Printf("[UpdateCheck] Is current version valid semver?: %v", semver.IsValid(currentVersion))

	client := http.Client{Timeout: time.Second * 10}

	url := "https://api.github.com/repos/Dncky35/claviger/releases"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "Claviger-Zero-Trust-Gateway")

	res, err := client.Do(req)
	if err != nil {
		// log.Printf("[UpdateCheck] ❌ HTTP Request failed: %v", err)
		return false, "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// log.Printf("[UpdateCheck] ❌ GitHub API returned non-200 status: %d", res.StatusCode)
		return false, "", fmt.Errorf("GitHub API returned status: %d", res.StatusCode)
	}

	var releases []GithubRelease
	if err := json.NewDecoder(res.Body).Decode(&releases); err != nil {
		// log.Printf("[UpdateCheck] ❌ Failed to decode JSON: %v", err)
		return false, "", err
	}

	// log.Printf("[UpdateCheck] 📥 Successfully fetched %d releases from GitHub", len(releases))

	highestStable := currentVersion
	highestPreRelease := currentVersion

	// 2. Iterate through all releases to categorize them
	for _, release := range releases {
		// isValid := semver.IsValid(release.TagName)
		// log.Printf("[UpdateCheck] Evaluating Tag: '%s' | Prerelease flag: %v | Valid SemVer: %v", release.TagName, release.Prerelease, isValid)

		if release.Prerelease {
			// Compare Beta/Alpha tags
			if semver.Compare(release.TagName, highestPreRelease) > 0 {
				// log.Printf("[UpdateCheck] 📈 New highest Beta found: '%s' (beats '%s')", release.TagName, highestPreRelease)
				highestPreRelease = release.TagName
			}
		} else {
			// Compare Stable tags
			if semver.Compare(release.TagName, highestStable) > 0 {
				// log.Printf("[UpdateCheck] 📈 New highest Stable found: '%s' (beats '%s')", release.TagName, highestStable)
				highestStable = release.TagName
			}
		}
	}

	hasStableUpdate := highestStable != currentVersion

	// A pre-release only matters if it is strictly newer than the current version AND newer than the highest stable version.
	hasPreReleaseUpdate := (highestPreRelease != currentVersion) && (semver.Compare(highestPreRelease, highestStable) > 0)

	// log.Printf("[UpdateCheck] --- FINAL VERDICT ---")
	// log.Printf("[UpdateCheck] Highest Stable: '%s' (Update available? %v)", highestStable, hasStableUpdate)
	// log.Printf("[UpdateCheck] Highest Beta: '%s' (Update available? %v)", highestPreRelease, hasPreReleaseUpdate)

	// 3. Store STABLE state in DB
	if hasStableUpdate {
		// log.Printf("[UpdateCheck] 💾 Saving Stable update to DB: '%s'", highestStable)
		storage.SetConfig(db, "available_update_version", highestStable)
	} else {
		storage.SetConfig(db, "available_update_version", "")
	}

	// 4. Store PRE-RELEASE state in DB for the UI
	if hasPreReleaseUpdate {
		// log.Printf("[UpdateCheck] 💾 Saving Beta update to DB: '%s'", highestPreRelease)
		storage.SetConfig(db, "available_prerelease_update_version", highestPreRelease)
	} else {
		storage.SetConfig(db, "available_prerelease_update_version", "")
	}

	return hasStableUpdate, highestStable, nil
}

// ApplyUpdate triggers the bash installation script for a specific version.
// This can be called via the UI or a Cron task.
func ApplyUpdate(targetVersion string) error {
	db := storage.InitDB()
	defer db.Close()

	log.Printf("🚀 Initiating system update to version: %s", targetVersion)

	// 1. Update the database state BEFORE the daemon is killed by the update script.
	// This ensures that when the systemd service boots back up, the UI immediately sees the new version.
	err := storage.SetConfig(db, "installed_version", targetVersion)
	if err != nil {
		return fmt.Errorf("failed to save new version state: %v", err)
	}

	// Clear the pending update flag so the UI stops showing the "Update Available" button
	storage.SetConfig(db, "available_update_version", "")

	// 2. Construct the CloudROcean Installer URL dynamically
	// Example: https://api.cloudrocean.com/v1/installers/claviger-server/v0.3.16b7
	installerURL := fmt.Sprintf("https://api.cloudrocean.com/v1/installers/claviger-server/%s", targetVersion)

	// 3. Execute the Bash Update Script
	// We use sh -c to pipe the curl output directly into bash.
	cmdStr := fmt.Sprintf("curl -sSL %s | sudo bash", installerURL)
	cmd := exec.Command("sh", "-c", cmdStr)

	// Note: We use cmd.Start() instead of cmd.Run()!
	// The bash script will run `systemctl stop claviger-server`. If we use Run() (which waits for completion),
	// the script would kill this very Go process before returning, potentially causing a messy exit.
	// Start() launches it in the background asynchronously.
	err = cmd.Start()
	if err != nil {
		// Rollback DB state if the command fails to even start
		storage.SetConfig(db, "installed_version", CurrentVersion)
		return fmt.Errorf("failed to launch update script: %v", err)
	}

	log.Println("✅ Update script launched successfully in the background. The daemon will restart momentarily.")

	return nil
}
