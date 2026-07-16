package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/bin/3xui-lite/internal/gen"
	"github.com/bin/3xui-lite/internal/models"
	"github.com/bin/3xui-lite/internal/xray"
	"github.com/google/uuid"
)

// GET /api/gen/defaults?preset=vless-reality|vless-tcp|ss2022
// Returns auto-filled fields without writing to DB (for form fill / preview).
func (s *Server) genDefaults(w http.ResponseWriter, r *http.Request) {
	preset := r.URL.Query().Get("preset")
	if preset == "" {
		preset = "vless-reality"
	}
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))
	used := s.usedPorts()
	draft, err := s.buildQuickDraft(preset, port, used)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": draft})
}

// POST /api/quick  body: {preset, port?, remark?, addClient?}
// One-click create inbound (+ default client) and apply core config.
func (s *Server) quickSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Preset    string `json:"preset"`
		Port      int    `json:"port"`
		Remark    string `json:"remark"`
		AddClient *bool  `json:"addClient"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if body.Preset == "" {
		body.Preset = "vless-reality"
	}
	addClient := true
	if body.AddClient != nil {
		addClient = *body.AddClient
	}

	used := s.usedPorts()
	draft, err := s.buildQuickDraft(body.Preset, body.Port, used)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if body.Remark != "" {
		draft.Remark = body.Remark
	}

	settingsRaw, _ := json.Marshal(draft.Settings)
	ib := models.Inbound{
		Remark:   draft.Remark,
		Enable:   true,
		Protocol: draft.Protocol,
		Listen:   draft.Listen,
		Port:     draft.Port,
		Network:  draft.Network,
		Security: draft.Security,
		Settings: string(settingsRaw),
		Tag:      fmt.Sprintf("inbound-%d", draft.Port),
	}
	if err := s.DB.Create(&ib).Error; err != nil {
		writeErr(w, 400, "创建入站失败: "+err.Error())
		return
	}

	var client *models.Client
	var link string
	if addClient && draft.Protocol != models.ProtoRelay {
		c := models.Client{
			InboundID: ib.ID,
			Email:     draft.ClientEmail,
			Enable:    true,
			UUID:      draft.ClientUUID,
			Password:  draft.ClientPassword,
			Flow:      draft.ClientFlow,
		}
		if c.Email == "" {
			c.Email = "user1"
		}
		if draft.Protocol == models.ProtoVLESS && c.UUID == "" {
			c.UUID = uuid.NewString()
		}
		if draft.Protocol == models.ProtoSS2022 && c.Password == "" {
			c.Password = gen.RandomBase64(16)
		}
		if err := s.DB.Create(&c).Error; err != nil {
			writeErr(w, 400, "创建客户端失败: "+err.Error())
			return
		}
		client = &c
		// host 可能仍是 127.0.0.1；前端/客户端请求 link 接口时会再按请求 Host 生成
		link = xray.ShareLink(s.getPublicHost(), ib, c)
	}

	warn := ""
	if err := s.applyCore(); err != nil {
		warn = err.Error()
	}

	// reload with clients
	_ = s.DB.Preload("Clients").First(&ib, ib.ID)

	writeJSON(w, 200, map[string]any{
		"ok":     true,
		"warn":   warn,
		"data":   ib,
		"client": client,
		"link":   link,
		"draft":  draft,
	})
}

// QuickDraft is the auto-generated plan for an inbound.
type QuickDraft struct {
	Preset         string         `json:"preset"`
	Remark         string         `json:"remark"`
	Protocol       string         `json:"protocol"`
	Listen         string         `json:"listen"`
	Port           int            `json:"port"`
	Network        string         `json:"network"`
	Security       string         `json:"security"`
	Settings       map[string]any `json:"settings"`
	ClientEmail    string         `json:"clientEmail"`
	ClientUUID     string         `json:"clientUuid"`
	ClientPassword string         `json:"clientPassword"`
	ClientFlow     string         `json:"clientFlow"`
	Tips           string         `json:"tips"`
}

func (s *Server) usedPorts() map[int]bool {
	var list []models.Inbound
	s.DB.Select("port").Find(&list)
	m := map[int]bool{}
	for _, ib := range list {
		m[ib.Port] = true
	}
	// panel / api reserved-ish
	m[18080] = true
	m[10085] = true
	return m
}

func (s *Server) buildQuickDraft(preset string, preferPort int, used map[int]bool) (*QuickDraft, error) {
	d := &QuickDraft{
		Preset:      preset,
		Listen:      "0.0.0.0",
		ClientEmail: "user1",
	}

	switch preset {
	case "vless-tcp", "vless":
		d.Remark = "一键-VLESS-TCP"
		d.Protocol = models.ProtoVLESS
		d.Network = "tcp"
		d.Security = "none"
		d.Port = gen.FreePort(firstPort(preferPort, 10443), used)
		d.ClientUUID = uuid.NewString()
		d.Settings = map[string]any{"flow": ""}
		d.Tips = "无 TLS 的本地/内网测试配置，外网请用 VLESS Reality。"

	case "vless-ws":
		d.Remark = "一键-VLESS-WS"
		d.Protocol = models.ProtoVLESS
		d.Network = "ws"
		d.Security = "none"
		d.Port = gen.FreePort(firstPort(preferPort, 8080), used)
		d.ClientUUID = uuid.NewString()
		path := gen.RandomPath()
		d.Settings = map[string]any{
			"flow": "",
			"path": path,
			"host": "",
		}
		d.Tips = "WebSocket 传输，可配合 CDN；当前未启用 TLS。"

	case "vless-reality", "reality":
		d.Remark = "一键-VLESS-Reality"
		d.Protocol = models.ProtoVLESS
		d.Network = "tcp"
		d.Security = "reality"
		// Prefer 8443 over 443: fewer admin/permission issues on Windows, less conflict with IIS/system.
		d.Port = gen.FreePort(firstPort(preferPort, 8443), used)
		d.ClientUUID = uuid.NewString()
		d.ClientFlow = "xtls-rprx-vision"
		priv, pub, err := gen.RealityKeyPairFromXray(s.Cores.Xray.Bin())
		if err != nil {
			return nil, err
		}
		sid := gen.ShortID()
		sni := "www.microsoft.com"
		d.Settings = map[string]any{
			"flow":              "xtls-rprx-vision",
			"realityPrivateKey": priv,
			"realityPublicKey":  pub,
			"serverNames":       []string{sni},
			"dest":              sni + ":443",
			"shortIds":          []string{sid},
			"fingerprint":       "chrome",
		}
		d.Tips = "推荐外网使用：TCP + Reality + Vision，密钥/ShortId 已自动生成。"

	case "ss2022", "ss":
		d.Remark = "一键-SS2022"
		d.Protocol = models.ProtoSS2022
		d.Network = "tcp"
		d.Security = "none"
		d.Port = gen.FreePort(firstPort(preferPort, 10888), used)
		serverKey := gen.RandomBase64(16) // aes-128
		userKey := gen.RandomBase64(16)
		d.ClientPassword = userKey
		d.Settings = map[string]any{
			"method":   "2022-blake3-aes-128-gcm",
			"password": serverKey,
			"network":  "tcp,udp",
		}
		d.Tips = "SS2022 服务端/用户密钥已自动生成（base64）。"

	default:
		return nil, fmt.Errorf("未知预设: %s（可用: vless-reality / vless-tcp / vless-ws / ss2022）", preset)
	}

	if d.Port == 0 {
		return nil, fmt.Errorf("无法分配可用端口")
	}
	return d, nil
}

func firstPort(prefer, fallback int) int {
	if prefer > 0 {
		return prefer
	}
	return fallback
}
