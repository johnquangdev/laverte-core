package admin

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"
)

type OverviewHandler struct {
	uc        overviewuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newOverviewHandler(uc overviewuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *OverviewHandler {
	return &OverviewHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *OverviewHandler) summary(c echo.Context) error {
	now := time.Now()
	// Default window: month-to-date. `to` is exclusive, so it lands on tomorrow
	// to include everything booked for today.
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	if raw := c.QueryParam("from"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, now.Location())
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from, want YYYY-MM-DD"})
		}
		from = parsed
	}
	if raw := c.QueryParam("to"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, now.Location())
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to, want YYYY-MM-DD"})
		}
		to = parsed
	}

	resp, err := h.uc.Summary(c.Request().Context(), from, to)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
