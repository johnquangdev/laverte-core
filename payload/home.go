package payload

type CreateHomeRequest struct {
	Name        string `json:"name" validate:"required"`
	Category    string `json:"category" validate:"required,oneof=home nest"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

// UpdateHomeRequest replaces the descriptive fields outright, but IsActive and
// GoogleCalendarID are pointers on purpose: for a plain value an omitted JSON key
// is indistinguishable from an explicit zero, so an admin editing only the address
// would silently take the home out of service and blank its calendar link. Nil
// means "leave as-is".
type UpdateHomeRequest struct {
	Name             string  `json:"name" validate:"required"`
	Category         string  `json:"category" validate:"required,oneof=home nest"`
	Address          string  `json:"address"`
	Description      string  `json:"description"`
	GoogleCalendarID *string `json:"google_calendar_id"`
	IsActive         *bool   `json:"is_active"`
}
