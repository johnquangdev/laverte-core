package payload

// GrantAdminRequest names the user by email or by id. Email is what a
// superadmin actually has in hand — the person signs in once with Google, then
// says which address they used; user_id stays for callers that already know it.
type GrantAdminRequest struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email" validate:"omitempty,email,max=254"`
}
