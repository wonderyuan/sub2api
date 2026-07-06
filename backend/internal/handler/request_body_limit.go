package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

type requestBodyLimitResponder func(status int, code string, message string)

// rejectIfAccountRequestBodyTooLarge treats an account body-size limit as a terminal
// client error for the selected account rather than a retryable upstream failure.
func rejectIfAccountRequestBodyTooLarge(
	reqLog *zap.Logger,
	account *service.Account,
	bodyBytes int64,
	respond requestBodyLimitResponder,
) bool {
	if account == nil || bodyBytes <= 0 {
		return false
	}
	limit := account.GetRequestBodyLimitBytes()
	if limit <= 0 || bodyBytes <= limit {
		return false
	}
	if reqLog != nil {
		reqLog.Warn("gateway.request_body_limit_exceeded",
			zap.Int64("account_id", account.ID),
			zap.Int64("request_body_bytes", bodyBytes),
			zap.Int64("request_body_limit_bytes", limit),
		)
	}
	respond(http.StatusRequestEntityTooLarge, "request_body_too_large", requestBodyLimitMessage(bodyBytes, limit))
	return true
}

func releaseAcquiredAccountSelection(selection *service.AccountSelectionResult) {
	if selection != nil && selection.Acquired && selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func requestBodyLimitMessage(bodyBytes, limit int64) string {
	return fmt.Sprintf("Request body too large: %d bytes exceeds account limit %d bytes", bodyBytes, limit)
}

func extractMaxBytesError(err error) (*http.MaxBytesError, bool) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return maxErr, true
	}
	return nil, false
}

func buildBodyTooLargeMessage(limit int64) string {
	return fmt.Sprintf("Request body too large: maximum size is %d bytes", limit)
}
