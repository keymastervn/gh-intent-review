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

module.exports = { calculateOrderTotal };
