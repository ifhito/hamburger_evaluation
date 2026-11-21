DROP TABLE IF EXISTS user_sessions;

DROP INDEX IF EXISTS users_email_lower_key;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));
