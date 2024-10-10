package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/golang/mock/gomock"
	"github.com/hairutdin/alexios/internal/store"
	"github.com/hairutdin/alexios/internal/store/mock"
	"github.com/stretchr/testify/assert"
)

func TestWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock.NewMockStore(ctrl)

	messages := []store.Message{
		{
			Sender:  "411419e5-f5be-4cdb-83aa-2ca2b6648353",
			Time:    time.Now(),
			Payload: "Hello!",
		},
	}

	s.EXPECT().
		ListMessages(gomock.Any(), gomock.Any()).
		Return(messages, nil)

	appInstance := newApp(s)

	handler := http.HandlerFunc(appInstance.webhook)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	testCases := []struct {
		name         string
		method       string
		body         string
		expectedCode int
		expectedBody string
	}{
		{
			name:         "method_get",
			method:       http.MethodGet,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_put",
			method:       http.MethodPut,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_delete",
			method:       http.MethodDelete,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_post_without_body",
			method:       http.MethodPost,
			expectedCode: http.StatusBadRequest,
			expectedBody: "",
		},
		{
			name:         "method_post_unsupported_type",
			method:       http.MethodPost,
			body:         `{"request": {"type": "idunno", "command": "do something"}, "version": "1.0"}`,
			expectedCode: http.StatusUnprocessableEntity,
			expectedBody: "",
		},
		{
			name:         "method_post_success",
			method:       http.MethodPost,
			body:         `{"request": {"type": "SimpleUtterance", "command": "sudo do something"}, "session": {"new": true}, "version": "1.0"}`,
			expectedCode: http.StatusOK,
			expectedBody: `Exact time .* hours, .* minutes. There are 1 new messages for you.`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			req := resty.New().R()
			req.Method = tc.method
			req.URL = srv.URL

			if len(tc.body) > 0 {
				req.SetHeader("Content-Type", "application/json")
				req.SetBody(tc.body)
			}

			resp, err := req.Send()
			assert.NoError(t, err, "error making HTTP request")

			assert.Equal(t, tc.expectedCode, resp.StatusCode(), "Response code didn't match expected")
			if tc.expectedBody != "" {
				assert.Regexp(t, tc.expectedBody, string(resp.Body()))
			}
		})
	}
}
