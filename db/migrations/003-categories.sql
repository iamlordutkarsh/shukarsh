ALTER TABLE products ADD COLUMN category TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (003, '003-categories');
