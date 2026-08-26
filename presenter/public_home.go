package presenter

import "github.com/johnquangdev/laverte-home/model"

type PublicHomeResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

func ToPublicHomeResponse(h *model.Home) PublicHomeResponse {
	return PublicHomeResponse{
		ID: h.ID, Name: h.Name, Category: h.Category,
		Address: h.Address, Description: h.Description,
	}
}
