package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func sendResend(apiKey, from, to string, notif Notification) error {
	subject := fmt.Sprintf("[%s] Claviger Alert: %s", notif.Level, notif.Title)

	payload := resendPayload{
		From:    from,
		To:      []string{to},
		Subject: subject,
		Text:    fmt.Sprintf("Alert Level: %s\nTitle: %s\n\n%s", notif.Level, notif.Title, notif.Message),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend API returned status code %d", resp.StatusCode)
	}

	return nil
}
