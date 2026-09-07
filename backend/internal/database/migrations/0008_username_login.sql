ALTER TABLE users ADD COLUMN username TEXT;

UPDATE users
SET username = 'user_' || substr(replace(id, '-', ''), 1, 12)
WHERE username IS NULL OR trim(username) = '';

CREATE UNIQUE INDEX users_username_unique_idx ON users(username COLLATE NOCASE);
