-- Add extra images and description fields
ALTER TABLE products ADD COLUMN images TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN long_description TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (004, '004-product-images');
