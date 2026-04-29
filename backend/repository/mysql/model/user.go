package model

type User struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"column:name;not null"`
	Email     string `gorm:"column:email;type:varchar(255);uniqueIndex;not null"`
	Password  string `gorm:"column:password;not null"`
	Avatar    string `gorm:"column:avatar"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime"`
	DeleteAt  int64  `gorm:"column:delete_at;index"`
}
