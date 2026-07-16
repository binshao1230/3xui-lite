package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bin/3xui-lite/internal/core"
	"github.com/bin/3xui-lite/internal/models"
	"github.com/bin/3xui-lite/internal/version"
	"github.com/bin/3xui-lite/internal/xray"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const panelVersion = version.Version

type Server struct {
	DB     *gorm.DB
	Cores  *core.Switch
	Static fs.FS
	Host   string // public host for share links
}

func New(db *gorm.DB, sw *core.Switch, static fs.FS) *Server {
	return &Server{DB: db, Cores: sw, Static: static, Host: "127.0.0.1"}
}

func (s *Server) Router() http.Handler {
	r := mux.NewRouter()

	r.HandleFunc("/api/login", s.login).Methods(http.MethodPost)
	r.HandleFunc("/api/logout", s.auth(s.logout)).Methods(http.MethodPost)
	r.HandleFunc("/api/status", s.auth(s.status)).Methods(http.MethodGet)
	r.HandleFunc("/api/password", s.auth(s.changePassword)).Methods(http.MethodPost)

	r.HandleFunc("/api/inbounds", s.auth(s.listInbounds)).Methods(http.MethodGet)
	r.HandleFunc("/api/inbounds", s.auth(s.createInbound)).Methods(http.MethodPost)
	r.HandleFunc("/api/quick", s.auth(s.quickSetup)).Methods(http.MethodPost)
	r.HandleFunc("/api/gen/defaults", s.auth(s.genDefaults)).Methods(http.MethodGet)
	r.HandleFunc("/api/inbounds/{id}", s.auth(s.getInbound)).Methods(http.MethodGet)
	r.HandleFunc("/api/inbounds/{id}", s.auth(s.updateInbound)).Methods(http.MethodPut)
	r.HandleFunc("/api/inbounds/{id}", s.auth(s.deleteInbound)).Methods(http.MethodDelete)
	r.HandleFunc("/api/inbounds/{id}/clients", s.auth(s.addClient)).Methods(http.MethodPost)
	r.HandleFunc("/api/inbounds/{id}/clients/{cid}", s.auth(s.updateClient)).Methods(http.MethodPut)
	r.HandleFunc("/api/inbounds/{id}/clients/{cid}", s.auth(s.deleteClient)).Methods(http.MethodDelete)
	r.HandleFunc("/api/inbounds/{id}/clients/{cid}/link", s.auth(s.clientLink)).Methods(http.MethodGet)
	r.HandleFunc("/api/inbounds/{id}/clients/{cid}/qrcode", s.auth(s.clientQRCode)).Methods(http.MethodGet)

	// Core control (preferred)
	r.HandleFunc("/api/core", s.auth(s.getCore)).Methods(http.MethodGet)
	r.HandleFunc("/api/core", s.auth(s.setCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/core/restart", s.auth(s.restartCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/core/stop", s.auth(s.stopCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/core/start", s.auth(s.startCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/core/logs", s.auth(s.coreLogs)).Methods(http.MethodGet)
	r.HandleFunc("/api/core/config", s.auth(s.previewConfig)).Methods(http.MethodGet)

	// Backward-compatible aliases
	r.HandleFunc("/api/xray/restart", s.auth(s.restartCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/xray/stop", s.auth(s.stopCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/xray/start", s.auth(s.startCore)).Methods(http.MethodPost)
	r.HandleFunc("/api/xray/logs", s.auth(s.coreLogs)).Methods(http.MethodGet)
	r.HandleFunc("/api/xray/config", s.auth(s.previewConfig)).Methods(http.MethodGet)

	r.HandleFunc("/api/settings/host", s.auth(s.getHost)).Methods(http.MethodGet)
	r.HandleFunc("/api/settings/host", s.auth(s.setHost)).Methods(http.MethodPost)

	// Online update from GitHub Releases
	r.HandleFunc("/api/update/check", s.auth(s.checkUpdate)).Methods(http.MethodGet)
	r.HandleFunc("/api/update", s.auth(s.applyUpdate)).Methods(http.MethodPost)

	// SPA
	r.PathPrefix("/").Handler(s.spaHandler())

	return withCORS(r)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) spaHandler() http.Handler {
	if s.Static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static not embedded", 500)
		})
	}
	fileServer := http.FileServer(http.FS(s.Static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if f, err := s.Static.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// fallback index
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "msg": msg})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" {
			// img/src 等无法带 Header 时，允许 query token
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			if c, err := r.Cookie("session"); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			writeErr(w, 401, "未登录")
			return
		}
		var sess models.Session
		if err := s.DB.Where("token = ? AND expires_at > ?", token, time.Now()).First(&sess).Error; err != nil {
			writeErr(w, 401, "会话无效或已过期")
			return
		}
		next(w, r)
	}
}

// ---------- auth ----------

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	var admin models.Admin
	if err := s.DB.Where("username = ?", body.Username).First(&admin).Error; err != nil {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(body.Password)) != nil {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	token := randToken(24)
	sess := models.Session{
		Token:     token,
		AdminID:   admin.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = s.DB.Create(&sess).Error
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	writeJSON(w, 200, map[string]any{"ok": true, "token": token, "username": admin.Username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	if token == "" {
		if c, err := r.Cookie("session"); err == nil {
			token = c.Value
		}
	}
	if token != "" {
		s.DB.Where("token = ?", token).Delete(&models.Session{})
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := readJSON(r, &body); err != nil || body.NewPassword == "" {
		writeErr(w, 400, "参数错误")
		return
	}
	var admin models.Admin
	if err := s.DB.First(&admin).Error; err != nil {
		writeErr(w, 500, "无管理员")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(body.OldPassword)) != nil {
		writeErr(w, 400, "原密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	admin.PasswordHash = string(hash)
	s.DB.Save(&admin)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- status ----------

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var inCount, clCount int64
	s.DB.Model(&models.Inbound{}).Count(&inCount)
	s.DB.Model(&models.Client{}).Count(&clCount)
	st := s.Cores.Status()
	am := s.Cores.ActiveManager()
	writeJSON(w, 200, map[string]any{
		"ok": true,
		"data": models.SystemStatus{
			ActiveCore:       s.Cores.Active(),
			CoreRunning:      am.IsRunning(),
			CoreVersion:      am.Version(),
			Uptime:           am.Uptime(),
			InboundCount:     int(inCount),
			ClientCount:      int(clCount),
			PanelVersion:     panelVersion,
			XrayRunning:      st["xrayRunning"].(bool),
			XrayVersion:      st["xrayVersion"].(string),
			XrayAvailable:    st["xrayAvailable"].(bool),
			SingboxRunning:   st["singboxRunning"].(bool),
			SingboxVersion:   st["singboxVersion"].(string),
			SingboxAvailable: st["singboxAvailable"].(bool),
		},
	})
}

// ---------- inbounds ----------

func (s *Server) listInbounds(w http.ResponseWriter, r *http.Request) {
	var list []models.Inbound
	s.DB.Preload("Clients").Order("id desc").Find(&list)
	writeJSON(w, 200, map[string]any{"ok": true, "data": list})
}

func (s *Server) getInbound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	var ib models.Inbound
	if err := s.DB.Preload("Clients").First(&ib, id).Error; err != nil {
		writeErr(w, 404, "入站不存在")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": ib})
}

type inboundReq struct {
	Remark         string          `json:"remark"`
	Enable         *bool           `json:"enable"`
	Protocol       string          `json:"protocol"`
	Listen         string          `json:"listen"`
	Port           int             `json:"port"`
	Network        string          `json:"network"`
	Security       string          `json:"security"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings"`
	Total          int64           `json:"total"`
	ExpiryTime     int64           `json:"expiryTime"`
}

func (s *Server) createInbound(w http.ResponseWriter, r *http.Request) {
	var req inboundReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		writeErr(w, 400, "端口无效")
		return
	}
	proto := req.Protocol
	if proto != models.ProtoVLESS && proto != models.ProtoSS2022 && proto != models.ProtoRelay {
		writeErr(w, 400, "仅支持 vless / ss2022 / relay")
		return
	}
	enable := true
	if req.Enable != nil {
		enable = *req.Enable
	}
	listen := req.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}
	tag := "inbound-" + strconv.Itoa(req.Port)
	ib := models.Inbound{
		Remark:         req.Remark,
		Enable:         enable,
		Protocol:       proto,
		Listen:         listen,
		Port:           req.Port,
		Network:        first(req.Network, "tcp"),
		Security:       first(req.Security, "none"),
		Settings:       string(req.Settings),
		StreamSettings: string(req.StreamSettings),
		Total:          req.Total,
		ExpiryTime:     req.ExpiryTime,
		Tag:            tag,
	}
	if ib.Settings == "null" || ib.Settings == "" {
		ib.Settings = "{}"
	}
	if err := s.DB.Create(&ib).Error; err != nil {
		writeErr(w, 400, "创建失败: "+err.Error())
		return
	}
	if err := s.applyCore(); err != nil {
		writeJSON(w, 200, map[string]any{"ok": true, "data": ib, "warn": "已保存但 Xray 应用失败: " + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": ib})
}

func (s *Server) updateInbound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	var ib models.Inbound
	if err := s.DB.First(&ib, id).Error; err != nil {
		writeErr(w, 404, "入站不存在")
		return
	}
	var req inboundReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if req.Remark != "" {
		ib.Remark = req.Remark
	}
	if req.Enable != nil {
		ib.Enable = *req.Enable
	}
	if req.Port > 0 {
		ib.Port = req.Port
		ib.Tag = "inbound-" + strconv.Itoa(req.Port)
	}
	if req.Listen != "" {
		ib.Listen = req.Listen
	}
	if req.Network != "" {
		ib.Network = req.Network
	}
	if req.Security != "" {
		ib.Security = req.Security
	}
	if len(req.Settings) > 0 && string(req.Settings) != "null" {
		ib.Settings = string(req.Settings)
	}
	if len(req.StreamSettings) > 0 && string(req.StreamSettings) != "null" {
		ib.StreamSettings = string(req.StreamSettings)
	}
	ib.Total = req.Total
	ib.ExpiryTime = req.ExpiryTime
	if err := s.DB.Save(&ib).Error; err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.applyCore(); err != nil {
		writeJSON(w, 200, map[string]any{"ok": true, "data": ib, "warn": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": ib})
}

func (s *Server) deleteInbound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	s.DB.Where("inbound_id = ?", id).Delete(&models.Client{})
	s.DB.Delete(&models.Inbound{}, id)
	_ = s.applyCore()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- clients ----------

type clientReq struct {
	Email      string `json:"email"`
	Enable     *bool  `json:"enable"`
	UUID       string `json:"uuid"`
	Flow       string `json:"flow"`
	Password   string `json:"password"`
	Total      int64  `json:"total"`
	ExpiryTime int64  `json:"expiryTime"`
}

func (s *Server) addClient(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	var ib models.Inbound
	if err := s.DB.First(&ib, id).Error; err != nil {
		writeErr(w, 404, "入站不存在")
		return
	}
	if ib.Protocol == models.ProtoRelay {
		writeErr(w, 400, "中转入站不支持客户端")
		return
	}
	var req clientReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	enable := true
	if req.Enable != nil {
		enable = *req.Enable
	}
	c := models.Client{
		InboundID:  ib.ID,
		Email:      req.Email,
		Enable:     enable,
		Flow:       req.Flow,
		Total:      req.Total,
		ExpiryTime: req.ExpiryTime,
	}
	switch ib.Protocol {
	case models.ProtoVLESS:
		c.UUID = req.UUID
		if c.UUID == "" {
			c.UUID = uuid.NewString()
		}
		if c.Email == "" {
			c.Email = c.UUID[:8]
		}
	case models.ProtoSS2022:
		c.Password = req.Password
		if c.Password == "" {
			c.Password = randToken(16)
		}
		if c.Email == "" {
			c.Email = c.Password[:8]
		}
	}
	if err := s.DB.Create(&c).Error; err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.applyCore(); err != nil {
		writeJSON(w, 200, map[string]any{"ok": true, "data": c, "warn": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": c})
}

func (s *Server) updateClient(w http.ResponseWriter, r *http.Request) {
	cid, _ := strconv.Atoi(mux.Vars(r)["cid"])
	var c models.Client
	if err := s.DB.First(&c, cid).Error; err != nil {
		writeErr(w, 404, "客户端不存在")
		return
	}
	var req clientReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if req.Email != "" {
		c.Email = req.Email
	}
	if req.Enable != nil {
		c.Enable = *req.Enable
	}
	if req.UUID != "" {
		c.UUID = req.UUID
	}
	if req.Flow != "" {
		c.Flow = req.Flow
	}
	if req.Password != "" {
		c.Password = req.Password
	}
	c.Total = req.Total
	c.ExpiryTime = req.ExpiryTime
	s.DB.Save(&c)
	_ = s.applyCore()
	writeJSON(w, 200, map[string]any{"ok": true, "data": c})
}

func (s *Server) deleteClient(w http.ResponseWriter, r *http.Request) {
	cid, _ := strconv.Atoi(mux.Vars(r)["cid"])
	s.DB.Delete(&models.Client{}, cid)
	_ = s.applyCore()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) clientLink(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	cid, _ := strconv.Atoi(mux.Vars(r)["cid"])
	ib, c, link, err := s.resolveClientLink(id, cid, r)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	host := s.publicHost(r)
	if link == "" {
		writeJSON(w, 200, map[string]any{
			"ok":       false,
			"msg":      "无法生成分享链接（检查协议/UUID/密码）",
			"host":     host,
			"protocol": ib.Protocol,
			"email":    c.Email,
			"hasUuid":  c.UUID != "",
			"hasPass":  c.Password != "",
		})
		return
	}
	resp := map[string]any{
		"ok":     true,
		"link":   link,
		"host":   host,
		"qrcode": fmt.Sprintf("/api/inbounds/%d/clients/%d/qrcode?size=280&token=", id, cid),
	}
	// 内嵌小图 base64（失败不阻塞链接）
	if png, qerr := qrPNG(link, 256); qerr == nil {
		resp["qrcodeData"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	writeJSON(w, 200, resp)
}

func (s *Server) clientQRCode(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	cid, _ := strconv.Atoi(mux.Vars(r)["cid"])
	_, _, link, err := s.resolveClientLink(id, cid, r)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	if link == "" {
		writeErr(w, 400, "该协议无分享链接")
		return
	}
	size := 320
	if v := r.URL.Query().Get("size"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 128 && n <= 1024 {
			size = n
		}
	}
	png, err := qrPNG(link, size)
	if err != nil {
		writeErr(w, 500, "二维码生成失败: "+err.Error())
		return
	}
	// ?format=json → base64 for SPA
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, 200, map[string]any{
			"ok":     true,
			"link":   link,
			"host":   s.publicHost(r),
			"qrcode": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(png)
}

// ---------- core ----------

// ApplyConfig rebuilds active-core config from DB and restarts the process.
func (s *Server) ApplyConfig() error {
	return s.applyCore()
}

func (s *Server) applyCore() error {
	var list []models.Inbound
	if err := s.DB.Preload("Clients").Find(&list).Error; err != nil {
		return err
	}
	return s.Cores.Apply(list)
}

func (s *Server) getCore(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"ok":   true,
		"data": s.Cores.Status(),
	})
}

func (s *Server) setCore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Core string `json:"core"` // xray | singbox
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	if body.Core != core.CoreXray && body.Core != core.CoreSingbox {
		writeErr(w, 400, "core 仅支持 xray 或 singbox")
		return
	}
	mgr := s.Cores.Manager(body.Core)
	if !mgr.Available() {
		writeErr(w, 400, body.Core+" 内核未安装: "+mgr.Bin())
		return
	}
	if err := s.Cores.SetActive(body.Core); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.DB.Save(&models.Setting{Key: "active_core", Value: body.Core})
	if err := s.applyCore(); err != nil {
		writeJSON(w, 200, map[string]any{
			"ok":   true,
			"core": body.Core,
			"warn": "已切换但启动失败: " + err.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "core": body.Core})
}

func (s *Server) restartCore(w http.ResponseWriter, r *http.Request) {
	if err := s.applyCore(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "core": s.Cores.Active()})
}

func (s *Server) startCore(w http.ResponseWriter, r *http.Request) {
	if err := s.applyCore(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "core": s.Cores.Active()})
}

func (s *Server) stopCore(w http.ResponseWriter, r *http.Request) {
	if err := s.Cores.ActiveManager().Stop(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) coreLogs(w http.ResponseWriter, r *http.Request) {
	m := s.Cores.ActiveManager()
	writeJSON(w, 200, map[string]any{
		"ok":        true,
		"core":      s.Cores.Active(),
		"data":      m.Logs(),
		"lastError": m.LastError(),
	})
}

func (s *Server) previewConfig(w http.ResponseWriter, r *http.Request) {
	var list []models.Inbound
	s.DB.Preload("Clients").Find(&list)
	name := r.URL.Query().Get("core")
	raw, err := s.Cores.Preview(list, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

// ---------- settings ----------

func (s *Server) getPublicHost() string {
	var st models.Setting
	if err := s.DB.First(&st, "key = ?", "public_host").Error; err == nil && st.Value != "" {
		return strings.TrimSpace(st.Value)
	}
	return s.Host
}

// publicHost 优先使用设置的公网域名/IP；未设置时从请求 Host 自动推断（VPS 常用）。
func (s *Server) publicHost(r *http.Request) string {
	if h := s.getPublicHost(); h != "" && h != "127.0.0.1" && h != "localhost" && h != "0.0.0.0" {
		// 去掉误填的协议与路径
		h = strings.TrimPrefix(h, "https://")
		h = strings.TrimPrefix(h, "http://")
		if i := strings.Index(h, "/"); i >= 0 {
			h = h[:i]
		}
		return h
	}
	if r == nil {
		return s.getPublicHost()
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	// X-Forwarded-Host 可能是 host:port 列表
	if i := strings.Index(host, ","); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "127.0.0.1" || host == "localhost" {
		return s.getPublicHost()
	}
	return host
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"ok":          true,
		"host":        s.getPublicHost(),
		"detectHost":  s.publicHost(r),
		"requestHost": r.Host,
	})
}

func (s *Server) setHost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host string `json:"host"`
	}
	if err := readJSON(r, &body); err != nil || body.Host == "" {
		writeErr(w, 400, "参数错误")
		return
	}
	host := strings.TrimSpace(body.Host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	st := models.Setting{Key: "public_host", Value: host}
	s.DB.Save(&st)
	s.Host = host
	writeJSON(w, 200, map[string]any{"ok": true, "host": host})
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Server) resolveClientLink(id, cid int, r *http.Request) (models.Inbound, models.Client, string, error) {
	var ib models.Inbound
	if err := s.DB.First(&ib, id).Error; err != nil {
		return ib, models.Client{}, "", fmt.Errorf("入站不存在")
	}
	var c models.Client
	if err := s.DB.Where("id = ? AND inbound_id = ?", cid, id).First(&c).Error; err != nil {
		return ib, c, "", fmt.Errorf("客户端不存在")
	}
	link := xray.ShareLink(s.publicHost(r), ib, c)
	return ib, c, link, nil
}

func qrPNG(content string, size int) ([]byte, error) {
	if content == "" {
		return nil, fmt.Errorf("empty content")
	}
	if size < 128 {
		size = 128
	}
	// 长链接用 Low 容错，提高生成成功率
	lvl := qrcode.Medium
	if len(content) > 200 {
		lvl = qrcode.Low
	}
	return qrcode.Encode(content, lvl, size)
}
