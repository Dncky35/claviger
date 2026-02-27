package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"claviger-server/models"
)

func Authenticate(setupKey, deviceID, deviceName, osName, arch string) string {
	reqData := models.RegisterRequest{
		SetupKey:   setupKey,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		DeviceOS:   osName,
		DeviceArch: arch,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		log.Fatalf("❌ Failed to encode JSON: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := "http://localhost:8000/v1/claviger/auth/register"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("❌ Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ Network error contacting Cloudrocean: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Fatalf("❌ API Rejected the Setup Key. Status: %d, Message: %s", resp.StatusCode, string(bodyBytes))
	}

	var resData models.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		log.Fatalf("❌ Failed to parse API response: %v", err)
	}

	return resData.APIToken
}
