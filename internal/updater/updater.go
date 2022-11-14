package updater

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Updater struct {
	DB     *gorm.DB
	Logger *zap.SugaredLogger
}
