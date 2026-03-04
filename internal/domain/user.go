package domain

import "time"

// UserRole 用户角色
type UserRole string

const (
	UserRoleAdmin  UserRole = "admin"
	UserRoleMember UserRole = "member"
)

// UserStatus 用户状态
type UserStatus string

const (
	UserStatusPending UserStatus = "pending"
	UserStatusActive  UserStatus = "active"
)

// User 用户
type User struct {
	ID        uint64     `json:"id"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 所属租户
	TenantID uint64 `json:"tenantID"`

	// 用户名（全局唯一登录标识）
	Username string `json:"username"`

	// bcrypt 密码哈希，不序列化
	PasswordHash string `json:"-"`

	// 角色
	Role UserRole `json:"role"`

	// 状态
	Status UserStatus `json:"status"`

	// 默认用户（迁移兼容）
	IsDefault bool `json:"isDefault"`

	// 最后登录时间
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
}

// DefaultUserID 默认用户 ID
const DefaultUserID uint64 = 1
