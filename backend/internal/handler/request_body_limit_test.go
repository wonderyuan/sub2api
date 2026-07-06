package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyLimitTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limit := int64(16)
	router := gin.New()
	router.Use(middleware.RequestBodyLimit(limit))
	router.POST("/test", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error": buildBodyTooLargeMessage(maxErr.Limit),
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "read_failed",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	payload := bytes.Repeat([]byte("a"), int(limit+1))
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), buildBodyTooLargeMessage(limit))
}

func TestRejectIfAccountRequestBodyTooLarge(t *testing.T) {
	tests := []struct {
		name       string
		account    *service.Account
		bodyBytes  int64
		wantReject bool
	}{
		{
			name:      "nil account does not reject",
			account:   nil,
			bodyBytes: 11,
		},
		{
			name:      "zero limit is unlimited",
			account:   &service.Account{Extra: map[string]any{service.RequestBodyLimitExtraKey: int64(0)}},
			bodyBytes: 11,
		},
		{
			name:      "equal to limit is accepted",
			account:   &service.Account{Extra: map[string]any{service.RequestBodyLimitExtraKey: int64(10)}},
			bodyBytes: 10,
		},
		{
			name:       "above limit is rejected",
			account:    &service.Account{Extra: map[string]any{service.RequestBodyLimitExtraKey: int64(10)}},
			bodyBytes:  11,
			wantReject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			rejected := rejectIfAccountRequestBodyTooLarge(nil, tt.account, tt.bodyBytes, func(status int, code string, message string) {
				called = true
				require.Equal(t, http.StatusRequestEntityTooLarge, status)
				require.Equal(t, "request_body_too_large", code)
				require.Contains(t, message, "11 bytes")
				require.Contains(t, message, "10 bytes")
			})

			require.Equal(t, tt.wantReject, rejected)
			require.Equal(t, tt.wantReject, called)
		})
	}
}

func TestRejectIfAccountRequestBodyTooLargeReleasesAcquiredSelection(t *testing.T) {
	released := 0
	selection := &service.AccountSelectionResult{
		Acquired: true,
		ReleaseFunc: func() {
			released++
		},
	}
	account := &service.Account{
		Extra: map[string]any{service.RequestBodyLimitExtraKey: int64(10)},
	}

	rejected := rejectIfAccountRequestBodyTooLarge(nil, account, 11, func(_ int, _ string, _ string) {
		releaseAcquiredAccountSelection(selection)
	})

	require.True(t, rejected)
	require.Equal(t, 1, released)
}
