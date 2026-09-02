package receipt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendReceiptModelsCourseDelivery(t *testing.T) {
	deadline := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	reportAt := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		order     Order
		wantError string
	}{
		{
			name: "sends receipt with access deadline and report date",
			order: Order{
				ID: "ord-2048", LearnerEmail: "learner@example.com", LearnerName: "Ari",
				CourseTitle: "Practical Algebra", AmountCents: 12900, Currency: "USD",
				CourseURL: "https://learn.example.com/courses/algebra", CompletionDeadline: deadline,
				EducatorReportAt: reportAt,
			},
		},
		{
			name: "rejects report scheduled before learner deadline",
			order: Order{
				ID: "ord-2049", LearnerEmail: "learner@example.com", CourseTitle: "Practical Algebra",
				CompletionDeadline: deadline, EducatorReportAt: deadline.Add(-24 * time.Hour),
			},
			wantError: "educator_report_at must be on or after completion_deadline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != emailSendPath {
					t.Errorf("path = %s, want %s", r.URL.Path, emailSendPath)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("missing bearer authorization")
				}
				if r.Header.Get("Idempotency-Key") != "receipt-ord-2048" {
					t.Errorf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if _, exists := payload["from"]; exists {
					t.Error("request should use the account default sender")
				}
				html, _ := payload["html"].(string)
				for _, want := range []string{"Practical Algebra", "129.00", "30 Sep 2026", "1 Oct 2026", "Open your course"} {
					if !strings.Contains(html, want) {
						t.Errorf("html missing %q", want)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"ok":true,"data":{"message_id":"msg-123"},"error":null,"metadata":{}}`)
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key", server.Client())
			got, err := client.SendReceipt(context.Background(), tt.order)
			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("error = %v, want %q", err, tt.wantError)
				}
				if calls != 0 {
					t.Fatalf("calls = %d, want 0", calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.MessageID != "msg-123" {
				t.Errorf("message_id = %q", got.MessageID)
			}
		})
	}
}

func TestSendReceiptRetriesRateLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`)
			return
		}
		io.WriteString(w, `{"ok":true,"data":{"message_id":"msg-after-retry"},"error":null,"metadata":{}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", server.Client())
	client.sleep = func(context.Context, time.Duration) error { return nil }
	order := Order{
		ID: "ord-retry", LearnerEmail: "learner@example.com", CourseTitle: "Geometry",
		CompletionDeadline: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		EducatorReportAt:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}

	got, err := client.SendReceipt(context.Background(), order)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || got.MessageID != "msg-after-retry" {
		t.Fatalf("calls = %d, message_id = %q", calls, got.MessageID)
	}
}
