package domain

import "time"

// Tenant 租户
type Tenant struct {
	ID        uint64     `json:"id"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 租户名称
	Name string `json:"name"`

	// URL-safe 唯一标识
	Slug string `json:"slug"`

	// 系统默认租户 (ID=1)
	IsDefault bool `json:"isDefault"`
}

// DefaultTenantID 默认租户 ID
const DefaultTenantID uint64 = 1

// TenantIDAll 哨兵值，表示不过滤租户，返回所有数据
const TenantIDAll uint64 = 0
