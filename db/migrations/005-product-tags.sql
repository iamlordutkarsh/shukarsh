-- Add tags for New Arrivals and Best Sellers
ALTER TABLE products ADD COLUMN is_new INTEGER NOT NULL DEFAULT 0;
ALTER TABLE products ADD COLUMN is_bestseller INTEGER NOT NULL DEFAULT 0;

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (005, '005-product-tags');
