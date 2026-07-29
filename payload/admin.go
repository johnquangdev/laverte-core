package payload

type GrantAdminRequest struct {
	UserID uint `json:"user_id" validate:"required"`
}
