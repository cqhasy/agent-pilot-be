package ioc

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/agent-pilot/agent-pilot-be/config"
	"github.com/agent-pilot/agent-pilot-be/pkg/logger"
	dao "github.com/agent-pilot/agent-pilot-be/repository/mysql"
)

func InitDB(conf *config.MysqlConfig, l logger.Logger) *gorm.DB {

	db, err := gorm.Open(mysql.Open(conf.Dsn), &gorm.Config{
		Logger: glogger.New(gormLoggerFunc(l.Debug), glogger.Config{
			SlowThreshold: 0,
			LogLevel:      glogger.Info,
		}),
	})
	if err != nil {
		panic(err)
	}
	//初始化所有表
	err = dao.InitTables(db)
	if err != nil {
		panic(err)
	}
	return db
}

type gormLoggerFunc func(msg string, fields ...logger.Field)

func (g gormLoggerFunc) Printf(s string, i ...interface{}) {
	formatedMsg := fmt.Sprintf(s, i...)
	g(formatedMsg, logger.Field{Key: "args", Val: i})
}
