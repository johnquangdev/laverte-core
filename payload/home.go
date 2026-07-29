package payload

type CreateHomeRequest struct {
	Name        string `json:"name" validate:"required"`
	Category    string `json:"category" validate:"required,oneof=home nest"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

type UpdateHomeRequest struct {
	Name             string `json:"name" validate:"required"`
	Category         string `json:"category" validate:"required,oneof=home nest"`
	Address          string `json:"address"`
	Description      string `json:"description"`
	GoogleCalendarID string `json:"google_calendar_id"`
	IsActive         bool   `json:"is_active"`
}
