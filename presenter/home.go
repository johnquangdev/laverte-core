package presenter

import "github.com/johnquangdev/laverte-home/model"

type HomeResponse struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Address          string `json:"address"`
	Description      string `json:"description"`
	GoogleCalendarID string `json:"google_calendar_id"`
	IsActive         bool   `json:"is_active"`
}

func ToHomeResponse(h *model.Home) HomeResponse {
	return HomeResponse{
		ID: h.ID, Name: h.Name, Category: h.Category, Address: h.Address,
		Description: h.Description, GoogleCalendarID: h.GoogleCalendarID, IsActive: h.IsActive,
	}
}
