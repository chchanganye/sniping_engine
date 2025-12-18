package model

type EmailSettings struct {
	Enabled  bool   `json:"enabled"`
	Email    string `json:"email"`
	AuthCode string `json:"authCode,omitempty"`
}
