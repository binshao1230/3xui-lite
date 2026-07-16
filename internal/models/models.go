package models

import "time"

// Admin is the panel login account.
type Admin struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Username     string `gorm:"uniqueIndex;size:64" json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Setting stores key/value panel options.
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `json:"value"`
}

// Protocol kinds supported by this lite panel.
const (
	ProtoVLESS = "vless"
	ProtoSS2022 = "ss2022"
	ProtoRelay = "relay" // 中转 (dokodemo-door -> outbound)
)

// Inbound is a listen entry managed by the panel.
type Inbound struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Remark     string    `gorm:"size:128" json:"remark"`
	Enable     bool      `gorm:"default:true" json:"enable"`
	Protocol   string    `gorm:"size:32;index" json:"protocol"` // vless | ss2022 | relay
	Listen     string    `gorm:"size:64;default:0.0.0.0" json:"listen"`
	Port       int       `gorm:"uniqueIndex" json:"port"`
	// Network / security (mainly for VLESS)
	Network    string    `gorm:"size:32;default:tcp" json:"network"` // tcp | ws | grpc
	Security   string    `gorm:"size:32;default:none" json:"security"` // none | tls | reality
	// Extra JSON blob for protocol-specific fields
	// VLESS: flow, reality (publicKey, privateKey, shortIds, serverNames, dest), path, host, serviceName
	// SS2022: method, password (server-level password if no per-client)
	// Relay: target address/port, network (tcp/udp)
	Settings   string    `gorm:"type:text" json:"settings"`
	StreamSettings string `gorm:"type:text" json:"streamSettings"`
	Sniffing   string    `gorm:"type:text" json:"sniffing"`
	// Traffic (bytes)
	Up         int64     `gorm:"default:0" json:"up"`
	Down       int64     `gorm:"default:0" json:"down"`
	Total      int64     `gorm:"default:0" json:"total"` // 0 = unlimited
	ExpiryTime int64     `gorm:"default:0" json:"expiryTime"` // unix ms, 0 = never
	Tag        string    `gorm:"size:64;uniqueIndex" json:"tag"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`

	Clients []Client `gorm:"foreignKey:InboundID;constraint:OnDelete:CASCADE" json:"clients,omitempty"`
}

// Client is a user under an inbound (VLESS uuid / SS2022 password).
type Client struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	InboundID  uint      `gorm:"index" json:"inboundId"`
	Email      string    `gorm:"size:128;index" json:"email"` // unique label used as stats email
	Enable     bool      `gorm:"default:true" json:"enable"`
	// VLESS
	UUID       string    `gorm:"size:64" json:"uuid"`
	Flow       string    `gorm:"size:64" json:"flow"`
	// SS2022
	Password   string    `gorm:"size:128" json:"password"`
	// Limits
	Up         int64     `gorm:"default:0" json:"up"`
	Down       int64     `gorm:"default:0" json:"down"`
	Total      int64     `gorm:"default:0" json:"total"`
	ExpiryTime int64     `gorm:"default:0" json:"expiryTime"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Session is a simple cookie session.
type Session struct {
	Token     string    `gorm:"primaryKey;size:64" json:"token"`
	AdminID   uint      `json:"adminId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// SystemStatus is returned by the dashboard API.
type SystemStatus struct {
	ActiveCore       string `json:"activeCore"` // xray | singbox
	CoreRunning      bool   `json:"coreRunning"`
	CoreVersion      string `json:"coreVersion"`
	Uptime           int64  `json:"uptime"`
	InboundCount     int    `json:"inboundCount"`
	ClientCount      int    `json:"clientCount"`
	PanelVersion     string `json:"panelVersion"`
	XrayRunning      bool   `json:"xrayRunning"`
	XrayVersion      string `json:"xrayVersion"`
	XrayAvailable    bool   `json:"xrayAvailable"`
	SingboxRunning   bool   `json:"singboxRunning"`
	SingboxVersion   string `json:"singboxVersion"`
	SingboxAvailable bool   `json:"singboxAvailable"`
}
