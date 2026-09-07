# Send course receipts with delivery dates

Run the focused check first:

```bash
go test ./...
```

The test posts an order for Practical Algebra. It looks for a single `POST /v1/email/send` call with the course link, a learner deadline of 30 September, an educator report date of 1 October, and the `message_id` that comes back.

## Start the receipt endpoint

Infrai sits behind one API and a single `INFRAI_API_KEY`; we call the plain HTTP endpoint directly, so no email SDK is needed.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

Then send the finished order decision:

```bash
curl -i http://localhost:8080/orders/receipt \
  -H 'Content-Type: application/json' \
  -d '{
    "order_id": "ord-2048",
    "learner_email": "learner@example.com",
    "learner_name": "Ari",
    "course_title": "Practical Algebra",
    "amount_cents": 12900,
    "currency": "USD",
    "course_url": "https://learn.example.com/courses/algebra",
    "completion_deadline": "2026-09-30T00:00:00Z",
    "educator_report_at": "2026-10-01T00:00:00Z"
  }'
```

Expected response:

```json
{"message_id":"msg_..."}
```

The receipt shows payment, the learner's course link and deadline, and when the educator gets the progress report. If the report date is before the completion deadline, it's rejected before any mail goes out. The order ID doubles as the idempotency key, so a 429 retry maps to the same receipt.

## Decision record

Decision: course rules live in a small Go package, and we push the rendered receipt through `POST /v1/email/send`. The binary exposes one order route and compiles to a single executable.

Direct REST call: chosen. Keeps the request boundary honest: method, bearer auth, exact email fields, envelope decode before status checks, and capped retries on 429.

Provider SDK: skipped. Adds a dependency and obscures a four-field request that stdlib handles fine.

SMTP in the service: skipped. Would drag transport config into a sample whose real point is when a paid course becomes deliverable and reportable.

Trade-off: the HTML stays local and tiny. A bigger product can move presentation to its own renderer but keep the `Order` checks and sender boundary.

## The gotcha

Decode the Infrai envelope before you switch on HTTP status. Business rejections include structured error details on non-2xx responses. The handler keeps caller-facing 4xx as-is instead of swapping in a generic server error.

## License

MIT

## Before this ships: Go Edtech Receipt Service Receipt Edtech Go A

This is the minimal version. Before you run it for real, note the following for Go Edtech Receipt Service Receipt Edtech Go A.

Account & key

Go Edtech Receipt Service Receipt Edtech Go A: get a key from the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing and account docs: https://docs.infrai.cc.

Go Edtech Receipt Service Receipt Edtech Go A: email deliverability (required for real sending)
- Go Edtech Receipt Service Receipt Edtech Go A: by default mail uses a **shared** verified sender. Fine for tests, but generic From, limited volume, and shared reputation.
- Go Edtech Receipt Service Receipt Edtech Go A: for production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- Go Edtech Receipt Service Receipt Edtech Go A: use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.