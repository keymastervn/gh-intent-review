# Test Workspaces

Two Node.js microservices with clean baseline code, plus a script that creates a real GitHub PR with suspicious changes for testing `gh-intent-review`.

## Services

| Service | Port | Responsibility |
|---------|------|----------------|
| `user-service` | 3001 | User CRUD, JWT auth |
| `order-service` | 3002 | Order placement, pricing, refunds |

Both services use PostgreSQL via `pg` with parameterised queries, env-var config, and no hardcoded secrets in the baseline.

## Create the test PR

```bash
cd examples/test_workspaces
./create_test_pr.sh [optional-repo-name]
```

The script:
1. Copies both services into a temp directory
2. Creates a new public GitHub repo and pushes the clean baseline
3. Branches and applies suspicious changes across both services
4. Opens a PR and prints the URL

Then run the review:

```bash
gh intent-review generate <PR-URL>
```

## Expected intents in the PR

| File | Intent | Issue |
|------|--------|-------|
| `user-service/handlers/user_handler.js` | `¿!` | SQL injection — `username` concatenated directly into query string |
| `user-service/handlers/user_handler.js` | `¿!` | Hardcoded Stripe secret key (`sk_live_...`) |
| `user-service/handlers/user_handler.js` | `¿!` | `password_hash` exposed — `SELECT *` returns full row to client |
| `user-service/handlers/user_handler.js` | `¿=` | Email validation regex duplicated verbatim in `deleteUser` |
| `order-service/handlers/order_handler.js` | `¿~` | N+1 query — individual `SELECT` per order inside a loop |
| `order-service/handlers/order_handler.js` | `¿&` | Hardcoded prod URL `http://user-service.prod.internal:3000` |
| `order-service/handlers/order_handler.js` | `¿?` | Nested ternary chain for validation — hard to read and maintain |
| `order-service/services/pricing_service.js` | `¿=` | Tax + discount logic duplicated in `calculateRefund` instead of reusing `calculateOrderTotal` |
