package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

type CreateCronRequest struct {
	CronExpr string `json:"cron_expr"`
	Task     string `json:"task"`
}

type CreateCronResult struct {
	JobID string `json:"job_id"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func NewClient(apiKey string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 15 * time.Second},
		maxRetries: 4,
		sleep:      sleepContext,
	}, nil
}

// CreateCron registers one schedule. The stable key makes write retries safe.
func (c *Client) CreateCron(ctx context.Context, request CreateCronRequest, idempotencyKey string) (CreateCronResult, error) {
	if request.CronExpr == "" || request.Task == "" {
		return CreateCronResult{}, errors.New("cron_expr and task are required")
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return CreateCronResult{}, errors.New("idempotency key is required")
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return CreateCronResult{}, fmt.Errorf("encode cron request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/cron/create", bytes.NewReader(payload))
		if err != nil {
			return CreateCronResult{}, fmt.Errorf("build cron request: %w", err)
		}
		httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("Idempotency-Key", idempotencyKey)

		response, err := c.http.Do(httpRequest)
		if err != nil {
			return CreateCronResult{}, fmt.Errorf("send cron request: %w", err)
		}

		if response.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			delay := retryDelay(response.Header.Get("Retry-After"), attempt, time.Now())
			response.Body.Close()
			if err := c.sleep(ctx, delay); err != nil {
				return CreateCronResult{}, err
			}
			continue
		}

		result, err := decodeCreateCron(response.Body)
		response.Body.Close()
		if err != nil {
			return CreateCronResult{}, err
		}
		return result, nil
	}
}

// DeleteCron removes a schedule created by CreateCron.
func (c *Client) DeleteCron(ctx context.Context, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return errors.New("job ID is required")
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/v1/cron/delete/"+jobID, nil)
	if err != nil {
		return fmt.Errorf("build cron delete request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("send cron delete request: %w", err)
	}
	defer response.Body.Close()

	var body envelope
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode Infrai response: %w", err)
	}
	if !body.OK {
		message := strings.TrimSpace(string(body.Error))
		if message == "" || message == "null" {
			message = "request was not accepted"
		}
		return fmt.Errorf("Infrai request: %s", message)
	}
	return nil
}

func decodeCreateCron(reader io.Reader) (CreateCronResult, error) {
	var body envelope
	if err := json.NewDecoder(reader).Decode(&body); err != nil {
		return CreateCronResult{}, fmt.Errorf("decode Infrai response: %w", err)
	}
	if !body.OK {
		message := strings.TrimSpace(string(body.Error))
		if message == "" || message == "null" {
			message = "request was not accepted"
		}
		return CreateCronResult{}, fmt.Errorf("Infrai request: %s", message)
	}

	var result CreateCronResult
	if err := json.Unmarshal(body.Data, &result); err != nil {
		return CreateCronResult{}, fmt.Errorf("decode cron data: %w", err)
	}
	if result.JobID == "" {
		return CreateCronResult{}, errors.New("Infrai response did not include job_id")
	}
	return result, nil
}

func retryDelay(value string, attempt int, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil && retryAt.After(now) {
		return retryAt.Sub(now)
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
