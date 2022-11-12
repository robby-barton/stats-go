package updater

import (
	"gorm.io/gorm"
)

type Updater struct {
	DB *gorm.DB
}
