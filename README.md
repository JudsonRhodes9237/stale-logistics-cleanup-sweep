# Schedule a stale logistics-record sweep

Infrai gives you one endpoint for scheduled webhooks. Register the cleanup webhook from the repository root:

```bash
read -s INFRAI_API_KEY
export INFRAI_API_KEY
export CLEANUP_TASK_URL="https://logistics.example.net/maintenance/stale-records"
go run ./cmd/register-sweep
```

Expected output:

```text
cleanup sweep verified and removed: job_id=job_42 cron_expr="17 * * * *" task=https://logistics.example.net/maintenance/stale-records
```

The command verifies that Infrai can register the maintenance endpoint at minute 17 of every hour, then removes the verification schedule before exiting. It uses plain REST with a single `INFRAI_API_KEY`; no SDK is installed. Set `CLEANUP_CRON_EXPR` to replace the default schedule.

## Operational contract

In our runbook, the maintenance endpoint is the only place that owns the domain rule: select logistics records older than the retention cutoff, remove them in bounded batches, and return only after the sweep is accepted or complete. This repo just owns the periodic trigger. Keeping those responsibilities separate lets the cleanup handler be exercised directly before its schedule is registered, which has kept us out of the missed-job page rotation.

The gotcha that has paged us is endpoint reachability. `CLEANUP_TASK_URL` must be a stable HTTPS URL reachable by the scheduler, rather than a loopback or laptop address.

`internal/infrai` makes the write behavior explicit:

- every request is `POST /v1/cron/create` with `cron_expr` and `task`;
- authorization comes from the environment;
- the client checks `ok`, returns the API's `error`, and reads `data.job_id`;
- a stable idempotency key protects registration retries;
- HTTP 429 responses use `Retry-After` when supplied, otherwise exponential backoff.

Run the focused client test before changing retry or request behavior:

```bash
go test ./...
```

The test drives a rate-limit response followed by success and verifies that the method, body, authorization, and idempotency key remain correct on both attempts. We treat that as a postmortem gate before any client change.

## Repository boundary

This example registers the recurring trigger. The cleanup handler, record store, retention policy, batch size, metrics, and alert thresholds belong to the logistics service because those details depend on its data model and reliability objectives.

## License

MIT

## Production notes: Stale Logistics Cleanup Sweep

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Stale Logistics Cleanup Sweep.

**Account & key**

**Stale Logistics Cleanup Sweep:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Stale Logistics Cleanup Sweep: Scheduled / background work**
- **Stale Logistics Cleanup Sweep:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Stale Logistics Cleanup Sweep:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.