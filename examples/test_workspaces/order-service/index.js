const express = require('express');
const { getOrder, getOrdersByUser, createOrder } = require('./handlers/order_handler');

const app = express();
app.use(express.json());

app.get('/orders/:id', getOrder);
app.get('/users/:userId/orders', getOrdersByUser);
app.post('/orders', createOrder);

app.get('/health', (_req, res) => res.json({ status: 'ok' }));

const PORT = process.env.PORT || 3002;
app.listen(PORT, () => console.log(`order-service listening on :${PORT}`));
