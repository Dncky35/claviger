package notifier

import (
	"context"
	"database/sql"
	"log"
)

var alertChan = make(chan Notification, 100)

// FireAlert pushes a new alert onto the channel without blocking the main caller
func FireAlert(level AlertLevel, title, message string) {
	select {
	case alertChan <- Notification{Level: level, Title: title, Message: message}:
	default:
		log.Println("⚠️ Alert channel full! Dropping notification:", title)
	}
}

// StartWorker runs in the background and processes alerts safely
func StartWorker(ctx context.Context, db *sql.DB) {
	log.Println("📣 Notifier Dispatcher Worker initialized.")

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("🛑 Stopping Notifier Dispatcher...")
				return

			case notif := <-alertChan:
				// Read current config directly from Key-Value table
				cfg := loadConfigFromDB(db)

				// --- 1. Telegram Dispatch ---
				if cfg.ShouldSendTelegram(notif.Level) {
					go func(n Notification) {
						if err := sendTelegram(cfg.TelegramBotToken, cfg.TelegramChatID, n); err != nil {
							log.Printf("❌ Failed to send Telegram alert: %v\n", err)
						} else {
							log.Printf("✅ Telegram alert sent: %s\n", n.Title)
						}
					}(notif)
				}

				// --- 2. Resend Dispatch ---
				if cfg.ShouldSendResend(notif.Level) {
					go func(n Notification) {
						if err := sendResend(cfg.ResendAPIKey, cfg.ResendFromEmail, cfg.ResendToEmail, n); err != nil {
							log.Printf("❌ Failed to send Resend email: %v\n", err)
						} else {
							log.Printf("✅ Resend email alert sent: %s\n", n.Title)
						}
					}(notif)
				}
			}
		}
	}()
}

// Helper to load settings from DB
func loadConfigFromDB(db *sql.DB) NotifierConfig {
	var cfg NotifierConfig

	getVal := func(key string) string {
		var val string
		_ = db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&val)
		return val
	}

	// Telegram
	cfg.TelegramEnabled = getVal("notif_telegram_enabled") == "true"
	cfg.TelegramBotToken = getVal("notif_telegram_bot_token")
	cfg.TelegramChatID = getVal("notif_telegram_chat_id")
	cfg.TelegramNotifyInfo = getVal("notif_telegram_notify_info") == "true"
	cfg.TelegramNotifyWarning = getVal("notif_telegram_notify_warning") == "true"
	cfg.TelegramNotifyCritical = getVal("notif_telegram_notify_critical") == "true"

	// Resend
	cfg.ResendEnabled = getVal("notif_resend_enabled") == "true"
	cfg.ResendAPIKey = getVal("notif_resend_api_key")
	cfg.ResendFromEmail = getVal("notif_resend_from_email")
	cfg.ResendToEmail = getVal("notif_resend_to_email")
	cfg.ResendNotifyInfo = getVal("notif_resend_notify_info") == "true"
	cfg.ResendNotifyWarning = getVal("notif_resend_notify_warning") == "true"
	cfg.ResendNotifyCritical = getVal("notif_resend_notify_critical") == "true"

	return cfg
}
