-- Page view tracking for analytics
CREATE TABLE IF NOT EXISTS page_views (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    product_id INTEGER,
    referrer TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    visitor_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_page_views_created_at ON page_views(created_at);
CREATE INDEX IF NOT EXISTS idx_page_views_path ON page_views(path);
CREATE INDEX IF NOT EXISTS idx_page_views_product_id ON page_views(product_id);

-- WhatsApp click tracking
CREATE TABLE IF NOT EXISTS wa_clicks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id INTEGER,
    click_type TEXT NOT NULL DEFAULT 'order',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (007, '007-page-views');
