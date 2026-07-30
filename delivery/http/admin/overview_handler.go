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
	// loc is the business's zone: a revenue range built in the host's zone instead is
	// shifted by the UTC offset at both ends, which for a UTC+7 business silently
	// moves seven hours of takings into the neighbouring period.
	loc *time.Location
}

func newOverviewHandler(uc overviewuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, loc *time.Location) *OverviewHandler {
	return &OverviewHandler{uc: uc, handleErr: handleErr, handleOK: handleOK, loc: loc}
}

func (h *OverviewHandler) summary(c echo.Context) error {
	now := time.Now().In(h.loc)
	// Default window: month-to-date. `to` is exclusive, so it lands on tomorrow
	// to include everything booked for today.
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, h.loc)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.loc).AddDate(0, 0, 1)

	if raw := c.QueryParam("from"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, h.loc)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from, want YYYY-MM-DD"})
		}
		from = parsed
	}
	if raw := c.QueryParam("to"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, h.loc)
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
