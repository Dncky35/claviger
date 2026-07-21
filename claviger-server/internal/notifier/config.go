package notifier

type AlertLevel string

const (
	LevelInfo     AlertLevel = "INFO"
	LevelWarning  AlertLevel = "WARNING"
	LevelCritical AlertLevel = "CRITICAL"
)

type Notification struct {
	Level   AlertLevel
	Title   string
	Message string
}

// NotifierConfig holds provider credentials and level routing switches
type NotifierConfig struct {
	// --- Telegram Configuration ---
	TelegramEnabled        bool
	TelegramBotToken       string
	TelegramChatID         string
	TelegramNotifyInfo     bool
	TelegramNotifyWarning  bool
	TelegramNotifyCritical bool

	// --- Resend (Email) Configuration ---
	ResendEnabled        bool
	ResendAPIKey         string
	ResendFromEmail      string
	ResendToEmail        string
	ResendNotifyInfo     bool
	ResendNotifyWarning  bool
	ResendNotifyCritical bool
}

// IsTelegramConfigured checks if credentials exist
func (c *NotifierConfig) IsTelegramConfigured() bool {
	return c.TelegramEnabled && c.TelegramBotToken != "" && c.TelegramChatID != ""
}

// IsResendConfigured checks if credentials exist
func (c *NotifierConfig) IsResendConfigured() bool {
	return c.ResendEnabled && c.ResendAPIKey != "" && c.ResendFromEmail != "" && c.ResendToEmail != ""
}

// ShouldSendTelegram evaluates if Telegram wants this specific alert level
func (c *NotifierConfig) ShouldSendTelegram(level AlertLevel) bool {
	if !c.IsTelegramConfigured() {
		return false
	}
	switch level {
	case LevelInfo:
		return c.TelegramNotifyInfo
	case LevelWarning:
		return c.TelegramNotifyWarning
	case LevelCritical:
		return c.TelegramNotifyCritical
	default:
		return false
	}
}

// ShouldSendResend evaluates if Resend wants this specific alert level
func (c *NotifierConfig) ShouldSendResend(level AlertLevel) bool {
	if !c.IsResendConfigured() {
		return false
	}
	switch level {
	case LevelInfo:
		return c.ResendNotifyInfo
	case LevelWarning:
		return c.ResendNotifyWarning
	case LevelCritical:
		return c.ResendNotifyCritical
	default:
		return false
	}
}
