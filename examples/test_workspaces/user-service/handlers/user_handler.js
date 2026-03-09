const db = require('../db/connection');

async function getUser(req, res) {
  const { id } = req.params;
  const result = await db.query(
    'SELECT id, name, email, created_at FROM users WHERE id = $1',
    [id]
  );
  if (!result.rows.length) {
    return res.status(404).json({ error: 'User not found' });
  }
  res.json(result.rows[0]);
}

async function searchUsers(req, res) {
  const { username } = req.query;
  const result = await db.query(
    'SELECT id, name, email FROM users WHERE username ILIKE $1 LIMIT 20',
    [`%${username}%`]
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
  await db.query('DELETE FROM users WHERE id = $1', [id]);
  res.status(204).send();
}

module.exports = { getUser, searchUsers, updateUser, deleteUser };
