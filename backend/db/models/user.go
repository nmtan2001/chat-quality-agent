package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           string    `gorm:"type:char(36);primaryKey" json:"id"`
	Email        string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	Name         string    `gorm:"type:varchar(255)" json:"name"`
	IsAdmin      bool      `gorm:"default:false" json:"is_admin"`
	TokenVersion int       `gorm:"default:0" json:"-"` // incremented on refresh to revoke old tokens
	Language     string    `gorm:"type:varchar(10);default:'vi'" json:"language"` // vi | en
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`

	Tenants []Tenant `gorm:"many2many:user_tenants;" json:"tenants,omitempty"`
}

type UserTenant struct {
	UserID   string `gorm:"type:char(36);primaryKey" json:"user_id"`
	TenantID    string `gorm:"type:char(36);primaryKey" json:"tenant_id"`
	Role        string `gorm:"type:varchar(20);default:'member'" json:"role"` // owner | admin | member
	Permissions string `gorm:"type:jsonb;default:'{}'" json:"permissions"`                 // JSON: {"channels":"rw","messages":"r","jobs":"rw","settings":"r"}
}

func (UserTenant) TableName() string {
	return "user_tenants"
}

// BeforeCreate hook to ensure JSON fields have valid values for PostgreSQL jsonb
func (ut *UserTenant) BeforeCreate(tx *gorm.DB) error {
	if ut.Permissions == "" {
		ut.Permissions = "{}"
	}
	return nil
}
