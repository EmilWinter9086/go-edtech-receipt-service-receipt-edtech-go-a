package receipt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const emailSendPath = "/v1/email/send"

type Order struct {
	ID                 string    `json:"order_id"`
	LearnerEmail       string    `json:"learner_email"`
	LearnerName        string    `json:"learner_name"`
	CourseTitle        string    `json:"course_title"`
	AmountCents        int       `json:"amount_cents"`
	Currency           string    `json:"currency"`
	CourseURL          string    `json:"course_url"`
	CompletionDeadline time.Time `json:"completion_deadline"`
	EducatorReportAt   time.Time `json:"educator_report_at"`
}

type Delivery struct {
	MessageID string `json:"message_id"`
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type emailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    httpClient,
		sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

var receiptTemplate = template.Must(template.New("receipt").Parse(`
<h1>Receipt for {{.CourseTitle}}</h1>
<p>Hi {{.LearnerName}}, payment of {{.Currency}} {{.Amount}} is confirmed for order {{.ID}}.</p>
<p><a href="{{.CourseURL}}">Open your course</a></p>
<p>Complete the course by {{.CompletionDeadline}}. Your educator receives the progress report on {{.EducatorReportAt}}.</p>
`))

func (c *Client) SendReceipt(ctx context.Context, order Order) (Delivery, error) {
	if err := validate(order); err != nil {
		return Delivery{}, err
	}

	var body bytes.Buffer
	err := receiptTemplate.Execute(&body, struct {
		Order
		Amount             string
		CompletionDeadline string
		EducatorReportAt   string
	}{
		Order:              order,
		Amount:             fmt.Sprintf("%.2f", float64(order.AmountCents)/100),
		CompletionDeadline: order.CompletionDeadline.Format("2 Jan 2006"),
		EducatorReportAt:   order.EducatorReportAt.Format("2 Jan 2006"),
	})
	if err != nil {
		return Delivery{}, fmt.Errorf("render receipt: %w", err)
	}

	payload := emailRequest{
		To:      order.LearnerEmail,
		Subject: "Course receipt: " + order.CourseTitle,
		HTML:    body.String(),
	}
	return c.send(ctx, payload, "receipt-"+order.ID)
}

func validate(order Order) error {
	switch {
	case strings.TrimSpace(order.ID) == "":
		return errors.New("order_id is required")
	case strings.TrimSpace(order.LearnerEmail) == "":
		return errors.New("learner_email is required")
	case strings.TrimSpace(order.CourseTitle) == "":
		return errors.New("course_title is required")
	case order.AmountCents < 0:
		return errors.New("amount_cents cannot be negative")
	case order.CompletionDeadline.IsZero():
		return errors.New("completion_deadline is required")
	case order.EducatorReportAt.Before(order.CompletionDeadline):
		return errors.New("educator_report_at must be on or after completion_deadline")
	default:
		return nil
	}
}

func (c *Client) send(ctx context.Context, payload emailRequest, idempotencyKey string) (Delivery, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Delivery{}, fmt.Errorf("encode email: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+emailSendPath, bytes.NewReader(body))
		if err != nil {
			return Delivery{}, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.http.Do(req)
		if err != nil {
			return Delivery{}, fmt.Errorf("send email: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return Delivery{}, fmt.Errorf("read email response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return Delivery{}, fmt.Errorf("decode email response: %w", err)
		}
		if !env.OK {
			apiErr := &APIError{HTTPStatus: res.StatusCode, Message: http.StatusText(res.StatusCode)}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				delay := retryDelay(res.Header.Get("Retry-After"), attempt)
				if err := c.sleep(ctx, delay); err != nil {
					return Delivery{}, err
				}
				continue
			}
			return Delivery{}, apiErr
		}
		if res.StatusCode >= 500 {
			return Delivery{}, fmt.Errorf("email service returned status %d", res.StatusCode)
		}

		var delivery Delivery
		if err := json.Unmarshal(env.Data, &delivery); err != nil {
			return Delivery{}, fmt.Errorf("decode delivery: %w", err)
		}
		if delivery.MessageID == "" {
			return Delivery{}, errors.New("email response omitted message_id")
		}
		return delivery, nil
	}
	return Delivery{}, errors.New("email retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}
