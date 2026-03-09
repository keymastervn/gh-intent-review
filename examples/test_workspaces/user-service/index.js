const express = require('express');
const { authenticate } = require('./middleware/authenticate');
const { getUser, searchUsers, updateUser, deleteUser } = require('./handlers/user_handler');

const app = express();
app.use(express.json());

app.get('/users/search', authenticate, searchUsers);
app.get('/users/:id', authenticate, getUser);
app.put('/users/:id', authenticate, updateUser);
app.delete('/users/:id', authenticate, deleteUser);

app.get('/health', (_req, res) => res.json({ status: 'ok' }));

const PORT = process.env.PORT || 3001;
app.listen(PORT, () => console.log(`user-service listening on :${PORT}`));
