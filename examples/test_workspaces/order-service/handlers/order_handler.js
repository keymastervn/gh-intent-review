const db = require('../db/connection');
const { calculateOrderTotal } = require('../services/pricing_service');

const USER_SERVICE_URL = process.env.USER_SERVICE_URL || 'http://localhost:3001';

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

  // Fetch orders and all their items in two queries to avoid N+1
  const ordersResult = await db.query(
    'SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC',
    [userId]
  );
  if (!ordersResult.rows.length) {
    return res.json([]);
  }

  const orderIds = ordersResult.rows.map((o) => o.id);
  const itemsResult = await db.query(
    'SELECT * FROM order_items WHERE order_id = ANY($1)',
    [orderIds]
  );

  const itemsByOrder = itemsResult.rows.reduce((acc, item) => {
    (acc[item.order_id] = acc[item.order_id] || []).push(item);
    return acc;
  }, {});

  const orders = ordersResult.rows.map((order) => ({
    ...order,
    items: itemsByOrder[order.id] || [],
  }));

  res.json(orders);
}

async function createOrder(req, res) {
  const { userId, items } = req.body;

  if (!userId || !Array.isArray(items) || items.length === 0) {
    return res.status(400).json({ error: 'userId and non-empty items are required' });
  }

  // Verify user exists before placing order
  const userResp = await fetch(`${USER_SERVICE_URL}/users/${userId}`, {
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
