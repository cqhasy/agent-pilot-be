package mysql

import (
	"github.com/agent-pilot/agent-pilot-be/repository/mysql/model"
	"gorm.io/gorm"
)

func InitTables(db *gorm.DB) error {
	return db.AutoMigrate(&model.User{})
}
