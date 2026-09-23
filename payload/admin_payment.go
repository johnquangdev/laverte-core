package payload

// NoteRequest carries the free-text record an admin leaves when closing out
// money by hand: which account the refund went to, or how a transfer was settled.
type NoteRequest struct {
	Note string `json:"note" validate:"required,max=500"`
}
