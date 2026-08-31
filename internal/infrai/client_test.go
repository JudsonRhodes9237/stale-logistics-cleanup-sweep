package infrai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateCronRetriesRateLimitWithSameIdempotencyKey(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer authorization")
		}
		if request.Header.Get("Idempotency-Key") != "logistics-stale-sweep-v1" {
			t.Fatal("idempotency key changed")
		}

		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 2 || body["cron_expr"] != "17 * * * *" || body["task"] != "https://logistics.example.net/maintenance/stale-records" {
			t.Fatalf("unexpected body: %#v", body)
		}

		writer.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			writer.Header().Set("Retry-After", "2")
			writer.WriteHeader(http.StatusTooManyRequests)
			writer.Write([]byte(`{"ok":false,"data":null,"error":{"message":"retry later"},"metadata":{}}`))
			return
		}
		writer.Write([]byte(`{"ok":true,"data":{"job_id":"job_42"},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client, err := NewClient("test-key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = server.URL
	client.http = server.Client()
	client.sleep = func(_ context.Context, delay time.Duration) error {
		if delay != 2*time.Second {
			t.Fatalf("delay = %s", delay)
		}
		return nil
	}

	result, err := client.CreateCron(context.Background(), CreateCronRequest{
		CronExpr: "17 * * * *",
		Task:     "https://logistics.example.net/maintenance/stale-records",
	}, "logistics-stale-sweep-v1")
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID != "job_42" || calls != 2 {
		t.Fatalf("result = %#v, calls = %d", result, calls)
	}
}
