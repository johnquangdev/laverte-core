package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID             uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Email          string         `gorm:"uniqueIndex;not null" json:"email"`
	Name           *string        `json:"name"`
	OAuthProvider  string         `gorm:"column:oauth_provider;not null" json:"oauth_provider"`
	OAuthID        string         `gorm:"column:oauth_id;not null" json:"oauth_id"`
	Phone          *string        `gorm:"uniqueIndex" json:"phone"`
	Role           string         `gorm:"default:'user'" json:"role"`
	AdminGrantedBy *uint          `json:"admin_granted_by"`
	AdminGrantedAt *time.Time     `json:"admin_granted_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type RefreshToken struct {
	ID        uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	TokenID   string     `gorm:"uniqueIndex;not null" json:"token_id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	FamilyID  string     `gorm:"not null;index" json:"family_id"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
}
