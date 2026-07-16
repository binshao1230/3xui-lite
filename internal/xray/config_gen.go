package xray

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bin/3xui-lite/internal/models"
)

// BuildConfig generates a full Xray config from enabled inbounds.
func BuildConfig(inbounds []models.Inbound) (map[string]any, error) {
	xInbounds := make([]map[string]any, 0, len(inbounds))
	for _, ib := range inbounds {
		if !ib.Enable {
			continue
		}
		obj, err := buildInbound(ib)
		if err != nil {
			return nil, fmt.Errorf("inbound %s (port %d): %w", ib.Remark, ib.Port, err)
		}
		xInbounds = append(xInbounds, obj)
	}

	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"StatsService", "HandlerService"},
		},
		"stats": map[string]any{},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
			"system": map[string]any{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"inbounds": append(xInbounds, map[string]any{
			"listen":   "127.0.0.1",
			"port":     10085,
			"protocol": "dokodemo-door",
			"settings": map[string]any{"address": "127.0.0.1"},
			"tag":      "api",
		}),
		"outbounds": []map[string]any{
			{"protocol": "freedom", "tag": "direct", "settings": map[string]any{}},
			{"protocol": "blackhole", "tag": "blocked", "settings": map[string]any{}},
		},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []map[string]any{
				{
					"type":        "field",
					"inboundTag":  []string{"api"},
					"outboundTag": "api",
				},
				{
					"type":        "field",
					"protocol":    []string{"bittorrent"},
					"outboundTag": "blocked",
				},
			},
		},
	}

	// Attach per-inbound relay outbounds if any
	extraOut, extraRules := buildRelayExtras(inbounds)
	if len(extraOut) > 0 {
		outs := cfg["outbounds"].([]map[string]any)
		cfg["outbounds"] = append(outs, extraOut...)
		routing := cfg["routing"].(map[string]any)
		rules := routing["rules"].([]map[string]any)
		routing["rules"] = append(extraRules, rules...)
	}

	return cfg, nil
}

func buildInbound(ib models.Inbound) (map[string]any, error) {
	tag := ib.Tag
	if tag == "" {
		tag = fmt.Sprintf("inbound-%d", ib.Port)
	}

	obj := map[string]any{
		"listen":   ib.Listen,
		"port":     ib.Port,
		"protocol": mapProtocol(ib.Protocol),
		"tag":      tag,
		"sniffing": defaultSniffing(ib.Sniffing),
	}

	switch ib.Protocol {
	case models.ProtoVLESS:
		settings, stream, err := buildVLESS(ib)
		if err != nil {
			return nil, err
		}
		obj["settings"] = settings
		obj["streamSettings"] = stream
	case models.ProtoSS2022:
		settings, err := buildSS2022(ib)
		if err != nil {
			return nil, err
		}
		obj["settings"] = settings
		obj["streamSettings"] = map[string]any{"network": "tcp"}
	case models.ProtoRelay:
		settings, err := buildRelay(ib)
		if err != nil {
			return nil, err
		}
		obj["settings"] = settings
		obj["streamSettings"] = map[string]any{"network": "tcp"}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", ib.Protocol)
	}
	return obj, nil
}

func mapProtocol(p string) string {
	switch p {
	case models.ProtoSS2022:
		return "shadowsocks"
	case models.ProtoRelay:
		return "dokodemo-door"
	default:
		return p
	}
}

func defaultSniffing(raw string) map[string]any {
	if raw != "" {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			return m
		}
	}
	return map[string]any{
		"enabled":      true,
		"destOverride": []string{"http", "tls", "quic"},
	}
}

// ---- VLESS ----

type vlessSettings struct {
	Flow       string `json:"flow"`
	Decryption string `json:"decryption"`
	// Reality / TLS extras stored in inbound.Settings
	RealityPrivateKey string   `json:"realityPrivateKey"`
	RealityPublicKey  string   `json:"realityPublicKey"`
	ShortIds          []string `json:"shortIds"`
	ServerNames       []string `json:"serverNames"`
	Dest              string   `json:"dest"`
	Fingerprint       string   `json:"fingerprint"`
	// Stream
	Path        string `json:"path"`
	Host        string `json:"host"`
	ServiceName string `json:"serviceName"`
}

func buildVLESS(ib models.Inbound) (map[string]any, map[string]any, error) {
	var extra vlessSettings
	if ib.Settings != "" {
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
	}

	clients := make([]map[string]any, 0)
	for _, c := range ib.Clients {
		if !c.Enable {
			continue
		}
		if c.UUID == "" {
			continue
		}
		item := map[string]any{
			"id":    c.UUID,
			"email": clientEmail(ib, c),
			"level": 0,
		}
		flow := c.Flow
		if flow == "" {
			flow = extra.Flow
		}
		if flow != "" {
			item["flow"] = flow
		}
		clients = append(clients, item)
	}

	settings := map[string]any{
		"clients":    clients,
		"decryption": "none",
	}

	network := ib.Network
	if network == "" {
		network = "tcp"
	}
	security := ib.Security
	if security == "" {
		security = "none"
	}

	stream := map[string]any{
		"network":  network,
		"security": security,
	}

	switch network {
	case "ws":
		ws := map[string]any{"path": firstNonEmpty(extra.Path, "/")}
		if extra.Host != "" {
			ws["headers"] = map[string]any{"Host": extra.Host}
		}
		stream["wsSettings"] = ws
	case "grpc":
		stream["grpcSettings"] = map[string]any{
			"serviceName": firstNonEmpty(extra.ServiceName, "grpc"),
		}
	case "tcp":
		stream["tcpSettings"] = map[string]any{"header": map[string]any{"type": "none"}}
	}

	if security == "reality" {
		shortIds := extra.ShortIds
		if len(shortIds) == 0 {
			shortIds = []string{""}
		}
		serverNames := extra.ServerNames
		if len(serverNames) == 0 {
			serverNames = []string{"www.cloudflare.com"}
		}
		dest := firstNonEmpty(extra.Dest, serverNames[0]+":443")
		stream["realitySettings"] = map[string]any{
			"show":        false,
			"dest":        dest,
			"xver":        0,
			"serverNames": serverNames,
			"privateKey":  extra.RealityPrivateKey,
			"shortIds":    shortIds,
		}
	} else if security == "tls" {
		// Minimal TLS placeholder — cert paths can be put in Settings later.
		var tls map[string]any
		if ib.StreamSettings != "" {
			_ = json.Unmarshal([]byte(ib.StreamSettings), &tls)
		}
		if tls == nil {
			tls = map[string]any{}
		}
		stream["tlsSettings"] = tls
	}

	return settings, stream, nil
}

// ---- SS2022 ----

type ssSettings struct {
	Method   string `json:"method"`   // e.g. 2022-blake3-aes-128-gcm
	Password string `json:"password"` // server key (base64)
	Network  string `json:"network"`  // tcp,udp
}

func buildSS2022(ib models.Inbound) (map[string]any, error) {
	var extra ssSettings
	if ib.Settings != "" {
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
	}
	method := firstNonEmpty(extra.Method, "2022-blake3-aes-128-gcm")
	network := firstNonEmpty(extra.Network, "tcp,udp")

	// Multi-user: clients as SS users; single-user: server password only.
	clients := make([]map[string]any, 0)
	for _, c := range ib.Clients {
		if !c.Enable || c.Password == "" {
			continue
		}
		clients = append(clients, map[string]any{
			"password": c.Password,
			"email":    clientEmail(ib, c),
			"level":    0,
		})
	}

	settings := map[string]any{
		"method":   method,
		"network":  network,
		"password": extra.Password,
	}
	if len(clients) > 0 {
		settings["clients"] = clients
	}
	return settings, nil
}

// ---- Relay (中转 / dokodemo-door) ----

type relaySettings struct {
	Address        string `json:"address"` // target host
	Port           int    `json:"port"`    // target port
	Network        string `json:"network"` // tcp | udp | tcp,udp
	FollowRedirect bool   `json:"followRedirect"`
	// Optional dedicated freedom outbound with domainStrategy
	DomainStrategy string `json:"domainStrategy"`
}

func buildRelay(ib models.Inbound) (map[string]any, error) {
	var extra relaySettings
	if ib.Settings != "" {
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
	}
	if extra.Address == "" || extra.Port == 0 {
		return nil, fmt.Errorf("relay requires address and port in settings")
	}
	network := firstNonEmpty(extra.Network, "tcp,udp")
	return map[string]any{
		"address":        extra.Address,
		"port":           extra.Port,
		"network":        network,
		"followRedirect": extra.FollowRedirect,
	}, nil
}

func buildRelayExtras(inbounds []models.Inbound) (outs []map[string]any, rules []map[string]any) {
	for _, ib := range inbounds {
		if !ib.Enable || ib.Protocol != models.ProtoRelay {
			continue
		}
		var extra relaySettings
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
		tag := ib.Tag
		if tag == "" {
			tag = fmt.Sprintf("inbound-%d", ib.Port)
		}
		outTag := "relay-out-" + tag
		// Use freedom so traffic goes to the address/port configured on dokodemo-door
		outs = append(outs, map[string]any{
			"protocol": "freedom",
			"tag":      outTag,
			"settings": map[string]any{
				"domainStrategy": firstNonEmpty(extra.DomainStrategy, "AsIs"),
			},
		})
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []string{tag},
			"outboundTag": outTag,
		})
	}
	return outs, rules
}

func clientEmail(ib models.Inbound, c models.Client) string {
	if c.Email != "" {
		return c.Email
	}
	return fmt.Sprintf("%s@%d", strings.ReplaceAll(c.UUID, "-", ""), ib.Port)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// MarshalConfig pretty-prints config JSON.
func MarshalConfig(cfg map[string]any) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}
