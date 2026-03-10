package service

import "github.com/awsl-project/maxx/internal/repository"

// NewTestAdminService creates an AdminService with only the invite code repo wired.
func NewTestAdminService(inviteRepo repository.InviteCodeRepository) *AdminService {
	return NewAdminService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		inviteRepo, // invite code repository
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		nil,
	)
}
