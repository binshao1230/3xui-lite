package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bin/3xui-lite/internal/api"
	"github.com/bin/3xui-lite/internal/config"
	"github.com/bin/3xui-lite/internal/core"
	"github.com/bin/3xui-lite/internal/database"
	"github.com/bin/3xui-lite/internal/models"
	"github.com/bin/3xui-lite/internal/proc"
	"github.com/bin/3xui-lite/web"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Default()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	xrayMgr := proc.New(proc.Options{
		Name:        "xray",
		Bin:         cfg.XrayBin,
		ConfigPath:  cfg.XrayConfig,
		RunArgs:     []string{"run", "-c", "{c}"},
		TestArgs:    []string{"run", "-test", "-c", "{c}"},
		VersionArgs: []string{"version"},
	})
	sbMgr := proc.New(proc.Options{
		Name:        "sing-box",
		Bin:         cfg.SingboxBin,
		ConfigPath:  cfg.SingboxConfig,
		RunArgs:     []string{"run", "-c", "{c}"},
		TestArgs:    []string{"check", "-c", "{c}"},
		VersionArgs: []string{"version"},
	})

	active := loadActiveCore(db)
	sw := core.NewSwitch(xrayMgr, sbMgr, active)

	webFS, err := web.Static()
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	srv := api.New(db, sw, webFS)

	go func() {
		if err := srv.ApplyConfig(); err != nil {
			log.Printf("[boot] core not started: %v", err)
		} else {
			log.Printf("[boot] active core started: %s", sw.Active())
		}
	}()

	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Router()}
	go func() {
		log.Printf("3xui-lite listening on http://%s", cfg.Listen)
		log.Printf("default login: admin / admin  (change it after login)")
		log.Printf("data: %s | xray: %s | sing-box: %s | active: %s",
			cfg.DataDir, cfg.XrayBin, cfg.SingboxBin, sw.Active())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	log.Println("shutting down...")
	sw.StopAll()
	_ = httpSrv.Close()
}

func loadActiveCore(db *gorm.DB) string {
	var st models.Setting
	if err := db.First(&st, "key = ?", "active_core").Error; err == nil {
		if st.Value == core.CoreSingbox || st.Value == core.CoreXray {
			return st.Value
		}
	}
	return core.CoreXray
}
