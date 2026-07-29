package model

const (
	RoleUser       = "user"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "superadmin"
)

func IsGrantableRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}

func HasAdminAccess(role string) bool {
	return role == RoleAdmin || role == RoleSuperAdmin
}
