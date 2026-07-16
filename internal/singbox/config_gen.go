package singbox

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bin/3xui-lite/internal/models"
)

// BuildConfig generates a sing-box config from enabled inbounds.
func BuildConfig(inbounds []models.Inbound) (map[string]any, error) {
	ins := make([]map[string]any, 0, len(inbounds))
	for _, ib := range inbounds {
		if !ib.Enable {
			continue
		}
		obj, err := buildInbound(ib)
		if err != nil {
			return nil, fmt.Errorf("inbound %s (port %d): %w", ib.Remark, ib.Port, err)
		}
		ins = append(ins, obj)
	}

	cfg := map[string]any{
		"log": map[string]any{
			"level":     "warn",
			"timestamp": true,
		},
		"inbounds": ins,
		"outbounds": []map[string]any{
			{"type": "direct", "tag": "direct"},
			{"type": "block", "tag": "block"},
		},
		"route": map[string]any{
			"rules": []map[string]any{
				{
					"protocol": "bittorrent",
					"outbound": "block",
				},
			},
			"final": "direct",
		},
	}
	return cfg, nil
}

func buildInbound(ib models.Inbound) (map[string]any, error) {
	tag := ib.Tag
	if tag == "" {
		tag = fmt.Sprintf("inbound-%d", ib.Port)
	}
	listen := ib.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}

	switch ib.Protocol {
	case models.ProtoVLESS:
		return buildVLESS(ib, tag, listen)
	case models.ProtoSS2022:
		return buildSS2022(ib, tag, listen)
	case models.ProtoRelay:
		return buildRelay(ib, tag, listen)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", ib.Protocol)
	}
}

type vlessSettings struct {
	Flow              string   `json:"flow"`
	RealityPrivateKey string   `json:"realityPrivateKey"`
	RealityPublicKey  string   `json:"realityPublicKey"`
	ShortIds          []string `json:"shortIds"`
	ServerNames       []string `json:"serverNames"`
	Dest              string   `json:"dest"`
	Fingerprint       string   `json:"fingerprint"`
	Path              string   `json:"path"`
	Host              string   `json:"host"`
	ServiceName       string   `json:"serviceName"`
}

func buildVLESS(ib models.Inbound, tag, listen string) (map[string]any, error) {
	var extra vlessSettings
	if ib.Settings != "" {
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
	}

	users := make([]map[string]any, 0)
	for _, c := range ib.Clients {
		if !c.Enable || c.UUID == "" {
			continue
		}
		u := map[string]any{
			"uuid": c.UUID,
			"name": clientName(ib, c),
		}
		flow := c.Flow
		if flow == "" {
			flow = extra.Flow
		}
		if flow != "" {
			u["flow"] = flow
		}
		users = append(users, u)
	}

	obj := map[string]any{
		"type":        "vless",
		"tag":         tag,
		"listen":      listen,
		"listen_port": ib.Port,
		"users":       users,
	}

	network := ib.Network
	if network == "" {
		network = "tcp"
	}
	switch network {
	case "ws":
		obj["transport"] = map[string]any{
			"type": "ws",
			"path": first(extra.Path, "/"),
			"headers": map[string]any{
				"Host": extra.Host,
			},
		}
	case "grpc":
		obj["transport"] = map[string]any{
			"type":         "grpc",
			"service_name": first(extra.ServiceName, "grpc"),
		}
	}

	security := ib.Security
	if security == "" {
		security = "none"
	}
	if security == "reality" {
		serverNames := extra.ServerNames
		if len(serverNames) == 0 {
			serverNames = []string{"www.cloudflare.com"}
		}
		destHost, destPort := parseDest(extra.Dest, serverNames[0], 443)
		shortIDs := extra.ShortIds
		if len(shortIDs) == 0 {
			shortIDs = []string{""}
		}
		obj["tls"] = map[string]any{
			"enabled":     true,
			"server_name": serverNames[0],
			"reality": map[string]any{
				"enabled": true,
				"handshake": map[string]any{
					"server":      destHost,
					"server_port": destPort,
				},
				"private_key": extra.RealityPrivateKey,
				"short_id":    shortIDs,
			},
		}
	} else if security == "tls" {
		// Certificate paths can be supplied via streamSettings later.
		obj["tls"] = map[string]any{
			"enabled": true,
		}
	}

	return obj, nil
}

type ssSettings struct {
	Method   string `json:"method"`
	Password string `json:"password"`
	Network  string `json:"network"`
}

func buildSS2022(ib models.Inbound, tag, listen string) (map[string]any, error) {
	var extra ssSettings
	if ib.Settings != "" {
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
	}
	method := first(extra.Method, "2022-blake3-aes-128-gcm")

	users := make([]map[string]any, 0)
	for _, c := range ib.Clients {
		if !c.Enable || c.Password == "" {
			continue
		}
		users = append(users, map[string]any{
			"name":     clientName(ib, c),
			"password": c.Password,
		})
	}

	obj := map[string]any{
		"type":        "shadowsocks",
		"tag":         tag,
		"listen":      listen,
		"listen_port": ib.Port,
		"method":      method,
		"password":    extra.Password,
	}
	if len(users) > 0 {
		obj["users"] = users
	}
	return obj, nil
}

type relaySettings struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Network string `json:"network"`
}

func buildRelay(ib models.Inbound, tag, listen string) (map[string]any, error) {
	var extra relaySettings
	if ib.Settings != "" {
		_ = json.Unmarshal([]byte(ib.Settings), &extra)
	}
	if extra.Address == "" || extra.Port == 0 {
		return nil, fmt.Errorf("relay requires address and port in settings")
	}
	// sing-box direct inbound with override acts as port forward (dokodemo-like).
	return map[string]any{
		"type":             "direct",
		"tag":              tag,
		"listen":           listen,
		"listen_port":      ib.Port,
		"override_address": extra.Address,
		"override_port":    extra.Port,
	}, nil
}

func clientName(ib models.Inbound, c models.Client) string {
	if c.Email != "" {
		return c.Email
	}
	if c.UUID != "" {
		return fmt.Sprintf("%s@%d", strings.ReplaceAll(c.UUID, "-", "")[:8], ib.Port)
	}
	return fmt.Sprintf("user-%d", c.ID)
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func parseDest(dest, fallbackHost string, fallbackPort int) (string, int) {
	if dest == "" {
		return fallbackHost, fallbackPort
	}
	if i := strings.LastIndex(dest, ":"); i > 0 {
		host := dest[:i]
		var port int
		if _, err := fmt.Sscanf(dest[i+1:], "%d", &port); err == nil && port > 0 {
			return host, port
		}
	}
	return dest, fallbackPort
}

func MarshalConfig(cfg map[string]any) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}
