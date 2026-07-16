package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bin/3xui-lite/internal/models"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(dbPath string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.Admin{},
		&models.Setting{},
		&models.Inbound{},
		&models.Client{},
		&models.Session{},
	); err != nil {
		return nil, err
	}
	if err := ensureDefaultAdmin(db); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureDefaultAdmin(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Admin{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	admin := models.Admin{Username: "admin", PasswordHash: string(hash)}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}
	fmt.Println("[3xui-lite] default admin created: admin / admin  (please change password)")
	return nil
}
