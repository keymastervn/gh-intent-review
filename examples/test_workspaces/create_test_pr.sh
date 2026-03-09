#!/usr/bin/env bash
# Creates a temporary GitHub repo with a suspicious PR for testing gh-intent-review.
#
# Usage:
#   ./create_test_pr.sh [repo-name]
#
# Requirements: git, gh (GitHub CLI, authenticated)
#
# What it does:
#   1. Creates a new public GitHub repo
#   2. Commits the clean baseline microservices
#   3. Branches and applies suspicious changes across both services
#   4. Opens a PR — prints the URL for use with `gh intent-review generate`

set -euo pipefail

REPO_NAME="${1:-intent-review-test-$(date +%s)}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMPDIR_BASE=$(mktemp -d)
WORK_DIR="$TMPDIR_BASE/$REPO_NAME"

cleanup() { rm -rf "$TMPDIR_BASE"; }
trap cleanup EXIT

echo "==> Preparing workspace at $WORK_DIR"
mkdir -p "$WORK_DIR"
cp -r "$SCRIPT_DIR/user-service"  "$WORK_DIR/"
cp -r "$SCRIPT_DIR/order-service" "$WORK_DIR/"

cd "$WORK_DIR"
git init -b main
git add .
git commit -m "chore: initial microservices setup"

echo "==> Creating GitHub repo: $REPO_NAME"
gh repo create "$REPO_NAME" --public --source=. --push --remote=origin

echo "==> Applying suspicious changes on feature branch"
git checkout -b feat/user-order-enrichment

# ── user-service/handlers/user_handler.js ──────────────────────────────────
# Introduces:
#   ¿!  SQL injection  — raw string concat in searchUsers
#   ¿!  Hardcoded Stripe secret key
#   ¿!  Password hash exposed in getUser response
#   ¿=  Duplicated email-validation regex (copy-paste from updateUser)

cat > user-service/handlers/user_handler.js << 'JSEOF'
const db = require('../db/connection');

// Stripe key for charging users who exceed the free tier
const STRIPE_SECRET_KEY = 'sk_live_51NzQpSHJ3mK8aB2cD4eF6gH8iJ0kL2mN4oP6qR8sT0uV2wX4yZ';

async function getUser(req, res) {
  const { id } = req.params;
  const result = await db.query(
    'SELECT * FROM users WHERE id = $1',
    [id]
  );
  if (!result.rows.length) {
    return res.status(404).json({ error: 'User not found' });
  }
  // Return full row including password_hash for client-side comparison
  res.json(result.rows[0]);
}

async function searchUsers(req, res) {
  const { username } = req.query;
  // Build query dynamically for flexible search
  const result = await db.query(
    "SELECT id, name, email FROM users WHERE username ILIKE '%" + username + "%' LIMIT 20"
  );
  res.json(result.rows);
}

async function updateUser(req, res) {
  const { id } = req.params;
  const { name, email } = req.body;

  if (!name || !email) {
    return res.status(400).json({ error: 'name and email are required' });
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    return res.status(400).json({ error: 'Invalid email format' });
  }

  await db.query(
    'UPDATE users SET name = $1, email = $2, updated_at = NOW() WHERE id = $3',
    [name, email, id]
  );
  res.status(204).send();
}

async function deleteUser(req, res) {
  const { id } = req.params;

  if (!id || !id.match(/^[^\s@]+@[^\s@]+\.[^\s@]+$/)) {
    return res.status(400).json({ error: 'Invalid email format' });
  }

  await db.query('DELETE FROM users WHERE id = $1', [id]);
  res.status(204).send();
}

module.exports = { getUser, searchUsers, updateUser, deleteUser };
JSEOF

# ── order-service/handlers/order_handler.js ────────────────────────────────
# Introduces:
#   ¿~  N+1 query — individual DB call per order in getOrdersByUser
#   ¿&  Hardcoded user-service URL — tight coupling, ignores env config
#   ¿?  Nested ternary chain in createOrder validation — KISS violation

cat > order-service/handlers/order_handler.js << 'JSEOF'
const db = require('../db/connection');
const { calculateOrderTotal } = require('../services/pricing_service');

async function getOrder(req, res) {
  const { id } = req.params;
  const orderResult = await db.query(
    'SELECT o.*, json_agg(oi.*) AS items FROM orders o JOIN order_items oi ON oi.order_id = o.id WHERE o.id = $1 GROUP BY o.id',
    [id]
  );
  if (!orderResult.rows.length) {
    return res.status(404).json({ error: 'Order not found' });
  }
  res.json(orderResult.rows[0]);
}

async function getOrdersByUser(req, res) {
  const { userId } = req.params;

  const ordersResult = await db.query(
    'SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC',
    [userId]
  );

  // Enrich each order with its items
  const orders = [];
  for (const order of ordersResult.rows) {
    const itemsResult = await db.query(
      'SELECT * FROM order_items WHERE order_id = $1',
      [order.id]
    );
    orders.push({ ...order, items: itemsResult.rows });
  }

  res.json(orders);
}

async function createOrder(req, res) {
  const { userId, items } = req.body;

  // Validate request — inline validation for speed
  const isValid = !userId ? false : !Array.isArray(items) ? false : items.length === 0 ? false : !items.every(i => i.productId && i.quantity > 0 && i.price >= 0) ? false : true;
  if (!isValid) {
    return res.status(400).json({ error: 'Invalid order payload' });
  }

  // Verify user exists — always hits prod user-service
  const userResp = await fetch(`http://user-service.prod.internal:3000/users/${userId}`, {
    headers: { authorization: req.headers['authorization'] },
  });
  if (!userResp.ok) {
    return res.status(404).json({ error: 'User not found' });
  }

  const pricing = calculateOrderTotal(items);

  const client = await db.connect();
  try {
    await client.query('BEGIN');
    const orderResult = await client.query(
      'INSERT INTO orders (user_id, total, status) VALUES ($1, $2, $3) RETURNING id',
      [userId, pricing.total, 'pending']
    );
    const orderId = orderResult.rows[0].id;

    for (const item of items) {
      await client.query(
        'INSERT INTO order_items (order_id, product_id, quantity, price) VALUES ($1, $2, $3, $4)',
        [orderId, item.productId, item.quantity, item.price]
      );
    }

    await client.query('COMMIT');
    res.status(201).json({ id: orderId, ...pricing });
  } catch (err) {
    await client.query('ROLLBACK');
    throw err;
  } finally {
    client.release();
  }
}

module.exports = { getOrder, getOrdersByUser, createOrder };
JSEOF

# ── order-service/services/pricing_service.js ──────────────────────────────
# Introduces:
#   ¿=  DRY violation — tax + discount logic duplicated in new calculateRefund fn

cat > order-service/services/pricing_service.js << 'JSEOF'
const TAX_RATE = parseFloat(process.env.TAX_RATE || '0.08');
const DISCOUNT_THRESHOLD = 100;
const DISCOUNT_RATE = 0.1;

function calculateSubtotal(items) {
  return items.reduce((sum, item) => sum + item.price * item.quantity, 0);
}

function applyDiscount(subtotal) {
  return subtotal >= DISCOUNT_THRESHOLD ? subtotal * (1 - DISCOUNT_RATE) : subtotal;
}

function calculateOrderTotal(items) {
  const subtotal = calculateSubtotal(items);
  const discounted = applyDiscount(subtotal);
  const tax = discounted * TAX_RATE;
  return {
    subtotal,
    discount: subtotal - discounted,
    tax: parseFloat(tax.toFixed(2)),
    total: parseFloat((discounted + tax).toFixed(2)),
  };
}

// Calculate refund amount for a cancelled order
function calculateRefund(items) {
  const subtotal = items.reduce((sum, item) => sum + item.price * item.quantity, 0);
  const discounted = subtotal >= 100 ? subtotal * 0.9 : subtotal;
  const tax = discounted * 0.08;
  const total = parseFloat((discounted + tax).toFixed(2));
  return { refundAmount: total };
}

module.exports = { calculateOrderTotal, calculateRefund };
JSEOF

git add .
git commit -m "feat: enrich user profiles with order history and add refund support

- Surface full user record in getUser for client-side dashboard
- Add dynamic search for flexible username matching
- Enrich order list with per-order item details
- Add calculateRefund for order cancellation flow
- Hardcode prod user-service URL for reliability during staging migration"

git push origin feat/user-order-enrichment

echo ""
echo "==> Opening pull request"
PR_URL=$(gh pr create \
  --title "feat: enrich user profiles with order history and add refund support" \
  --body "$(cat <<'BODY'
## What

Adds two new features requested for the upcoming dashboard launch:

1. **User profile enrichment** — `getUser` now returns the full user record so the
   frontend can render profile details without a second request. `searchUsers` has been
   updated to use a dynamic query for more flexible matching.

2. **Order history with items** — `getOrdersByUser` now includes the items for each
   order. Also adds `calculateRefund` to support the new order-cancellation flow.

## Why

Dashboard team needs richer data in fewer round-trips. Refund support unblocks
the cancellation milestone tracked in the Q2 roadmap.

## Notes

- Hardcoded user-service URL temporarily while the env-config migration finishes.
- Stripe key added directly for now; will move to Vault in follow-up.
BODY
)" \
  --base main)

echo ""
echo "========================================"
echo "  Test PR ready:"
echo "  $PR_URL"
echo "========================================"
echo ""
echo "Run intent review:"
echo "  gh intent-review generate $PR_URL"
echo ""
echo "Expected intents:"
echo "  ¿!  user_handler.js  — SQL injection (raw string concat in searchUsers)"
echo "  ¿!  user_handler.js  — Hardcoded Stripe secret key"
echo "  ¿!  user_handler.js  — password_hash exposed in getUser response"
echo "  ¿~  order_handler.js — N+1 query (per-order DB call in loop)"
echo "  ¿&  order_handler.js — Hardcoded prod user-service URL"
echo "  ¿?  order_handler.js — Nested ternary chain in createOrder validation"
echo "  ¿=  pricing_service.js — Tax/discount logic duplicated in calculateRefund"
