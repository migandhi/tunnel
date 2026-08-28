# Business Logic — Accounts, Plans, and Enforcement

How the service behaves as a *product*: who gets access, for how long, with
what limits, and what happens when limits are hit. Written for the operator.

## 1. The Account Model

One row in `users` = one customer/tunnel identity:

| Field | Business meaning |
|---|---|
| `subdomain` | The product itself — their permanent public URL (`https://<sub>.<domain>`). Unique, immutable after creation. |
| `token_hash` | Their credential. Server never stores the token itself. |
| `plan` | A **label** (`free`/`basic`/`pro`/`custom`) — informational only; actual entitlements are the fields below. |
| `active` | Master kill switch. |
| `expires_at` | Subscription end (created as `now + N days`). |
| `bandwidth_limit` | Bytes per billing period. `0` = unlimited. |
| `bandwidth_used` | Running usage this period. |
| `tcp_port` | `0` = HTTP-only plan; `>0` = includes a dedicated public TCP port. |
| `email` | Optional contact reference for the operator. |

**Design principle:** the plan name is cosmetic; *limits are explicit per
user*. This lets you cut custom deals without code changes.

## 2. Account Lifecycle & States

```
            create
              │
              ▼
        ┌──────────┐  client connects   ┌────────┐
        │ Offline  │◄──────────────────►│ Online │
        └────┬─────┘                    └───┬────┘
             │                              │
   expiry date passes ──────────────────────┤──► Expired    (auto)
   bandwidth_used ≥ limit ──────────────────┤──► Over limit (auto)
   admin clicks Disable ────────────────────┴──► Disabled   (manual)
             │
   Renew / Reset BW / Enable ──► back to Offline/Online
             │
   admin Delete ──► gone (subdomain + TCP port freed instantly)
```

Dashboard status logic (priority order): Disabled → Expired → Over limit →
Online → Offline.

## 3. Enforcement Rules (what happens, when)

Two enforcement points, same rules:

| Rule | At connect (handshake) | Mid-session (enforcer, every 30s) |
|---|---|---|
| `active = false` | Rejected: "account is disabled" | Live tunnel killed ≤ ~45s |
| `now ≥ expires_at` | Rejected with the exact expiry date | Killed ≤ ~45s |
| `used ≥ limit` (limit > 0) | Rejected | Killed ≤ ~45s |
| TCP requested, no port | Rejected: "plan has no TCP port" | n/a |

**Grace behavior — deliberate choices:**
- Expiry is a **hard cutoff** (no grace period). Grace = just renew for more days.
- An over-limit user may briefly exceed the limit (up to one 30s cycle +
  15s flush) — accepted slack, not billable precision.
- **Proactive warnings** are pushed to the client at connect: expiry within
  7 days, and bandwidth ≥ 80% used. Users are never surprised.

## 4. The Bandwidth "Billing Period"

There is no calendar cycle built in — the period is defined by admin actions:

- **Renew (with "reset BW" ticked)** = start a new period: new expiry, usage
  back to 0. This is the normal monthly action.
- **Reset BW** alone = mid-cycle top-up / goodwill reset.
- **Set limit** = upgrade/downgrade takes effect within one enforcer cycle.

Usage counts **both directions** of tunnel traffic. Remember your VPS
provider bills you for that transfer too — price plans accordingly.

## 5. Token Policy (credential business rules)

- Issued once, displayed once, unrecoverable — only resettable.
- **Reset = revocation**: old token fails instantly, live sessions are
  disconnected. Use it for: lost tokens, suspected leaks, offboarding a
  device, employee departure.
- One token per account. One live tunnel per account per protocol
  (a new connection displaces the old one — prevents token sharing from
  multiplying capacity).

## 6. TCP Ports as Inventory

- Fixed inventory: `TCP_PORT_MIN..MAX` (default 250 ports).
- Assigned at user creation (auto = lowest free), held for the account's
  lifetime, released only on delete.
- A port is a **premium entitlement** — natural upsell over HTTP-only plans.

## 7. Suggested Plan Structure (example, not enforced by code)

| Plan label | Days | Bandwidth | TCP port | Typical use |
|---|---|---|---|---|
| free | 7–14 | 5 GB | no | trials, demos |
| basic | 30 | 50 GB | no | dev/webhooks |
| pro | 30 | 200 GB | yes | game servers, SSH, always-on |
| custom | any | any / unlimited | optional | negotiated |

Capacity sanity for a 1 GB droplet: dozens of concurrent light tunnels is
comfortable; the binding constraints are VPS transfer quota and CPU under
sustained throughput — watch `/metrics` and provider bandwidth graphs.

## 8. What Is Deliberately NOT in the System

- **No payments/invoicing** — collect money however you like (manual, Stripe
  links, bank transfer) and reflect it via *Renew*. A future billing service
  can automate exactly the operations the dashboard exposes (create, renew,
  set-limit, disable) against the same store.
- **No self-service signup** — every account is admin-created. This is an
  abuse-prevention feature on a shared domain/IP.
- **No per-request pricing/metering** — only aggregate bytes.
- **No SLA machinery** — single-node service; set expectations accordingly.

## 9. Accountability

Every business-relevant event is in the audit log (`/audit`):
user create/renew/disable/enable/delete, token resets, limit changes,
bandwidth resets, tunnel connect/disconnect (with source IP), failed auth
attempts, and enforcer terminations with reasons. This is your record for
customer disputes ("was I really online?", "who used my bandwidth?") and for
abuse investigations.

## 10. Operator Playbook (business decisions → clicks)

| Situation | Action |
|---|---|
| Customer paid for a month | Renew 30 days + reset BW |
| Customer wants more data mid-month | Set limit ↑, or Reset BW |
| Payment overdue | Do nothing — expiry cuts them off automatically |
| Abuse report on a subdomain | Disable → investigate `/audit` → delete or re-enable |
| Suspected token leak | New token → deliver securely |
| Customer churned | Delete (frees subdomain + TCP port for resale) |
