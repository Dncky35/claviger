package models

// RegisterRequest represents the payload sent to FastAPI
type RegisterRequest struct {
	SetupKey   string `json:"setup_key"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	DeviceOS   string `json:"device_os"`
	DeviceArch string `json:"device_arch"`
}

type RegisterResponse struct {
	APIToken string `json:"api_token"`
}
