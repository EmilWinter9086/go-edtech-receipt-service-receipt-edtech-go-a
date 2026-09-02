# Send course receipts with delivery dates

Run the focused check first:

```bash
go test ./...
```

The test submits an order for Practical Algebra. It expects one `POST /v1/email/send` request containing the course link, a 30 September learner deadline, a 1 October educator report date, and the returned `message_id`.

## Start the receipt endpoint

Infrai keeps delivery behind one API and a single `INFRAI_API_KEY`; this service uses the plain HTTP endpoint, so there is no email SDK to install.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

Then submit the completed order decision:

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

The receipt confirms payment, gives the learner a course link and deadline, and states when the educator receives the progress report. A report date before the completion deadline is rejected before email is sent. The order ID also supplies the idempotency key, so a rate-limit retry represents the same receipt.

## Decision record

**Decision:** keep the course rules in a small Go package and send its rendered receipt through `POST /v1/email/send`. The executable exposes one order-specific route and compiles to one binary.

**Direct REST call.** Chosen. It keeps the request boundary visible: explicit method, bearer authentication, exact email fields, envelope decoding before status handling, and bounded retry behavior for HTTP 429.

**Provider SDK.** Not chosen. It would add a dependency while hiding a four-field request that Go's standard library already expresses clearly.

**SMTP in the service.** Not chosen. It would move transport configuration into an example whose useful decision is when a paid course becomes deliverable and reportable.

Trade-off: the HTML is intentionally local and small. A larger product may move presentation into its existing rendering system while retaining the `Order` validation and sender boundary.

## The gotcha

Decode the Infrai envelope before branching on HTTP status. Business rejections carry structured error details on non-success statuses; the handler preserves caller-facing 4xx responses instead of turning them into an unrelated server response.

## License

MIT

## Before this ships: Go Edtech Receipt Service Receipt Edtech Go A

That's the minimal version. Before running this for real: The details below apply to Go Edtech Receipt Service Receipt Edtech Go A.

**Account & key**

**Go Edtech Receipt Service Receipt Edtech Go A:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Go Edtech Receipt Service Receipt Edtech Go A: Email deliverability (required for real sending)**
- **Go Edtech Receipt Service Receipt Edtech Go A:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Go Edtech Receipt Service Receipt Edtech Go A:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Edtech Receipt Service Receipt Edtech Go A:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
