package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"example.com/logistics-cleanup-sweep/internal/infrai"
)

const (
	defaultSchedule = "17 * * * *"
	idempotencyKey  = "logistics-stale-record-sweep-v1"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	taskURL := os.Getenv("CLEANUP_TASK_URL")
	if taskURL == "" {
		log.Fatal("CLEANUP_TASK_URL is required")
	}
	schedule := os.Getenv("CLEANUP_CRON_EXPR")
	if schedule == "" {
		schedule = defaultSchedule
	}

	client, err := infrai.NewClient(apiKey)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	job, err := client.CreateCron(ctx, infrai.CreateCronRequest{
		CronExpr: schedule,
		Task:     taskURL,
	}, idempotencyKey)
	if err != nil {
		log.Fatal(err)
	}
	if err := client.DeleteCron(ctx, job.JobID); err != nil {
		log.Fatalf("delete cleanup sweep %s: %v", job.JobID, err)
	}

	fmt.Printf("cleanup sweep verified and removed: job_id=%s cron_expr=%q task=%s\n", job.JobID, schedule, taskURL)
}
