package api

import (
	"claviger-server/internal/notifier"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// NotificationConfigPayload represents the JSON payload for the UI.
type NotificationConfigPayload struct {
	// Telegram
	TelegramEnabled        bool   `json:"telegram_enabled"`
	TelegramBotToken       string `json:"telegram_bot_token"`
	TelegramChatID         string `json:"telegram_chat_id"`
	TelegramNotifyInfo     bool   `json:"telegram_notify_info"`
	TelegramNotifyWarning  bool   `json:"telegram_notify_warning"`
	TelegramNotifyCritical bool   `json:"telegram_notify_critical"`

	// Resend
	ResendEnabled        bool   `json:"resend_enabled"`
	ResendAPIKey         string `json:"resend_api_key"`
	ResendFromEmail      string `json:"resend_from_email"`
	ResendToEmail        string `json:"resend_to_email"`
	ResendNotifyInfo     bool   `json:"resend_notify_info"`
	ResendNotifyWarning  bool   `json:"resend_notify_warning"`
	ResendNotifyCritical bool   `json:"resend_notify_critical"`
}

func HandleGetNotifications(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := NotificationConfigPayload{}

		// Helper to fetch and parse string to boolean
		getBool := func(key string) bool {
			var val string
			_ = db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&val)
			return val == "true"
		}

		// Helper to fetch strings
		getString := func(key string) string {
			var val string
			_ = db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&val)
			return val
		}

		cfg.TelegramEnabled = getBool("notif_telegram_enabled")
		cfg.TelegramBotToken = getString("notif_telegram_bot_token")
		cfg.TelegramChatID = getString("notif_telegram_chat_id")
		cfg.TelegramNotifyInfo = getBool("notif_telegram_notify_info")
		cfg.TelegramNotifyWarning = getBool("notif_telegram_notify_warning")
		cfg.TelegramNotifyCritical = getBool("notif_telegram_notify_critical")

		cfg.ResendEnabled = getBool("notif_resend_enabled")
		cfg.ResendAPIKey = getString("notif_resend_api_key")
		cfg.ResendFromEmail = getString("notif_resend_from_email")
		cfg.ResendToEmail = getString("notif_resend_to_email")
		cfg.ResendNotifyInfo = getBool("notif_resend_notify_info")
		cfg.ResendNotifyWarning = getBool("notif_resend_notify_warning")
		cfg.ResendNotifyCritical = getBool("notif_resend_notify_critical")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	}
}

// HandleSaveNotifications saves the settings into the config table.
func HandleSaveNotifications(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload NotificationConfigPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// Prepare the upsert statement
		stmt, err := db.Prepare(`
			INSERT INTO config (key, value) VALUES (?, ?) 
			ON CONFLICT(key) DO UPDATE SET value = excluded.value;
		`)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		// Helper to save boolean as string
		saveBool := func(key string, val bool) {
			stmt.Exec(key, strconv.FormatBool(val))
		}

		// Helper to save string
		saveStr := func(key string, val string) {
			stmt.Exec(key, val)
		}

		// Save all fields
		saveBool("notif_telegram_enabled", payload.TelegramEnabled)
		saveStr("notif_telegram_bot_token", payload.TelegramBotToken)
		saveStr("notif_telegram_chat_id", payload.TelegramChatID)
		saveBool("notif_telegram_notify_info", payload.TelegramNotifyInfo)
		saveBool("notif_telegram_notify_warning", payload.TelegramNotifyWarning)
		saveBool("notif_telegram_notify_critical", payload.TelegramNotifyCritical)

		saveBool("notif_resend_enabled", payload.ResendEnabled)
		saveStr("notif_resend_api_key", payload.ResendAPIKey)
		saveStr("notif_resend_from_email", payload.ResendFromEmail)
		saveStr("notif_resend_to_email", payload.ResendToEmail)
		saveBool("notif_resend_notify_info", payload.ResendNotifyInfo)
		saveBool("notif_resend_notify_warning", payload.ResendNotifyWarning)
		saveBool("notif_resend_notify_critical", payload.ResendNotifyCritical)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}
}

func HandleTestNotification() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// We fire a test alert at all three levels to ensure matrix routing works.
		notifier.FireAlert(
			notifier.LevelInfo,
			"🔔 Test Alert (INFO)",
			"This is an info-level test from Claviger.",
		)

		notifier.FireAlert(
			notifier.LevelWarning,
			"🔔 Test Alert (WARNING)",
			"This is a warning-level test from Claviger.",
		)

		notifier.FireAlert(
			notifier.LevelCritical,
			"🔔 Test Alert (CRITICAL)",
			"This is a critical-level test from Claviger.",
		)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"test_dispatched"}`))
	}
}
