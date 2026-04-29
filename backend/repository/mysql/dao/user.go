package dao

import (
	"context"
	"time"

	"github.com/agent-pilot/agent-pilot-be/repository/mysql/model"
	"gorm.io/gorm"
)

type UserDaoInterface interface {
	CreateUser(ctx context.Context, user model.User) (model.User, error)
	GetUserByID(ctx context.Context, id uint64) (model.User, error)
	GetUserByEmail(ctx context.Context, email string) (model.User, error)
	UpdateUser(ctx context.Context, user model.User) (model.User, error)
	DeleteUser(ctx context.Context, id uint64) error
}

type UserDAO struct {
	DB *gorm.DB
}

func NewUserDao(db *gorm.DB) UserDaoInterface {
	return &UserDAO{
		DB: db,
	}
}

func (d *UserDAO) CreateUser(ctx context.Context, user model.User) (model.User, error) {
	err := d.DB.WithContext(ctx).Create(&user).Error
	if err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (d *UserDAO) GetUserByID(ctx context.Context, id uint64) (model.User, error) {
	var user model.User
	err := d.DB.WithContext(ctx).Where("delete_at = ?", 0).First(&user, id).Error
	if err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (d *UserDAO) GetUserByEmail(ctx context.Context, email string) (model.User, error) {
	var user model.User
	err := d.DB.WithContext(ctx).Where("email = ? AND delete_at = ?", email, 0).First(&user).Error
	if err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (d *UserDAO) UpdateUser(ctx context.Context, user model.User) (model.User, error) {
	var existing model.User
	if err := d.DB.WithContext(ctx).Where("delete_at = ?", 0).First(&existing, user.ID).Error; err != nil {
		return model.User{}, err
	}

	updates := make(map[string]interface{})
	if user.Name != "" {
		updates["name"] = user.Name
	}
	if user.Email != "" {
		updates["email"] = user.Email
	}
	if user.Password != "" {
		updates["password"] = user.Password
	}
	if user.Avatar != "" {
		updates["avatar"] = user.Avatar
	}
	if len(updates) == 0 {
		return existing, nil
	}

	if err := d.DB.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
		return model.User{}, err
	}

	return d.GetUserByID(ctx, user.ID)
}

func (d *UserDAO) DeleteUser(ctx context.Context, id uint64) error {
	now := time.Now().Unix()
	result := d.DB.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ? AND delete_at = ?", id, 0).
		Updates(map[string]interface{}{
			"delete_at":  now,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}
