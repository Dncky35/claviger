package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type telegramPayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func sendTelegram(token, chatID string, notif Notification) error {
	var emoji string
	switch notif.Level {
	case LevelInfo:
		emoji = "ℹ️"
	case LevelWarning:
		emoji = "⚠️"
	case LevelCritical:
		emoji = "🚨"
	}

	formattedMsg := fmt.Sprintf("%s *[%s] %s*\n\n%s", emoji, notif.Level, notif.Title, notif.Message)

	payload := telegramPayload{
		ChatID:    chatID,
		Text:      formattedMsg,
		ParseMode: "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status code %d", resp.StatusCode)
	}

	return nil
}
