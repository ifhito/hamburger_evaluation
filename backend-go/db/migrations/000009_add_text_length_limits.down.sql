ALTER TABLE users DROP CONSTRAINT users_email_max_length;
ALTER TABLE users DROP CONSTRAINT users_username_max_length;
ALTER TABLE shops DROP CONSTRAINT shops_moderation_note_max_length;
ALTER TABLE shops DROP CONSTRAINT shops_name_max_length;
ALTER TABLE burgers DROP CONSTRAINT burgers_name_max_length;
ALTER TABLE reviews DROP CONSTRAINT reviews_comment_max_length;
