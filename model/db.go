package model

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global MySQL instance, opened at startup.
var DB *gorm.DB

// InitDB opens a MySQL connection and verifies it with a ping.
// Schema is user-managed; no auto-migration is performed.
func InitDB(dsn string) error {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("mysql (%s): %w", dsn, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("mysql get underlying db: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("mysql ping (%s): %w", dsn, err)
	}
	DB = db
	return nil
}
