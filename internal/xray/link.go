package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/bin/3xui-lite/internal/models"
)

// ShareLink builds a single client share URI (no subscription aggregation).
func ShareLink(host string, ib models.Inbound, c models.Client) string {
	switch ib.Protocol {
	case models.ProtoVLESS:
		return vlessLink(host, ib, c)
	case models.ProtoSS2022:
		return ss2022Link(host, ib, c)
	default:
		return ""
	}
}

func vlessLink(host string, ib models.Inbound, c models.Client) string {
	var extra vlessSettings
	_ = json.Unmarshal([]byte(ib.Settings), &extra)

	u := url.URL{
		Scheme: "vless",
		User:   url.User(c.UUID),
		Host:   fmt.Sprintf("%s:%d", host, ib.Port),
	}
	q := url.Values{}
	q.Set("encryption", "none")
	network := ib.Network
	if network == "" {
		network = "tcp"
	}
	q.Set("type", network)
	security := ib.Security
	if security == "" {
		security = "none"
	}
	q.Set("security", security)

	flow := c.Flow
	if flow == "" {
		flow = extra.Flow
	}
	if flow != "" {
		q.Set("flow", flow)
	}

	switch network {
	case "ws":
		q.Set("path", firstNonEmpty(extra.Path, "/"))
		if extra.Host != "" {
			q.Set("host", extra.Host)
		}
	case "grpc":
		q.Set("serviceName", firstNonEmpty(extra.ServiceName, "grpc"))
	}

	if security == "reality" {
		if len(extra.ServerNames) > 0 {
			q.Set("sni", extra.ServerNames[0])
		}
		if extra.RealityPublicKey != "" {
			q.Set("pbk", extra.RealityPublicKey)
		}
		if len(extra.ShortIds) > 0 {
			q.Set("sid", extra.ShortIds[0])
		}
		fp := firstNonEmpty(extra.Fingerprint, "chrome")
		q.Set("fp", fp)
	}

	u.RawQuery = q.Encode()
	name := c.Email
	if name == "" {
		name = ib.Remark
	}
	u.Fragment = url.QueryEscape(name)
	// Fragment should not be double-encoded for most clients; use raw name.
	link := u.String()
	// Fix fragment
	if idx := strings.LastIndex(link, "#"); idx >= 0 {
		link = link[:idx+1] + url.PathEscape(name)
	}
	return link
}

func ss2022Link(host string, ib models.Inbound, c models.Client) string {
	var extra ssSettings
	_ = json.Unmarshal([]byte(ib.Settings), &extra)
	method := firstNonEmpty(extra.Method, "2022-blake3-aes-128-gcm")
	// SS2022: method:password@host:port  (user password may be server:user for multi-user)
	pass := c.Password
	if extra.Password != "" && c.Password != "" {
		// multi-user: serverKey:userKey
		pass = extra.Password + ":" + c.Password
	} else if pass == "" {
		pass = extra.Password
	}
	userInfo := method + ":" + pass
	encoded := base64.RawURLEncoding.EncodeToString([]byte(userInfo))
	// Some clients expect standard base64
	encodedStd := base64.StdEncoding.EncodeToString([]byte(userInfo))
	_ = encoded
	name := c.Email
	if name == "" {
		name = ib.Remark
	}
	return fmt.Sprintf("ss://%s@%s:%d#%s", encodedStd, host, ib.Port, url.PathEscape(name))
}
