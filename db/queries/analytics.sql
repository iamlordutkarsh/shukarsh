-- name: InsertPageView :exec
INSERT INTO page_views (path, product_id, referrer, user_agent, visitor_id)
VALUES (?, ?, ?, ?, ?);

-- name: InsertWAClick :exec
INSERT INTO wa_clicks (product_id, click_type) VALUES (?, ?);

-- name: ViewsPerDay :many
SELECT DATE(created_at) as day, COUNT(*) as views
FROM page_views
WHERE created_at >= datetime('now', '-30 days')
GROUP BY DATE(created_at)
ORDER BY day;

-- name: TopProducts :many
SELECT p.id, p.title, p.image_url, COUNT(pv.id) as views
FROM page_views pv
JOIN products p ON p.id = pv.product_id
WHERE pv.product_id IS NOT NULL AND pv.created_at >= datetime('now', '-30 days')
GROUP BY p.id
ORDER BY views DESC
LIMIT 10;

-- name: TotalViews :one
SELECT COUNT(*) as total FROM page_views;

-- name: TodayViews :one
SELECT COUNT(*) as total FROM page_views WHERE DATE(created_at) = DATE('now');

-- name: TotalWAClicks :one
SELECT COUNT(*) as total FROM wa_clicks;

-- name: TodayWAClicks :one
SELECT COUNT(*) as total FROM wa_clicks WHERE DATE(created_at) = DATE('now');

-- name: UniqueVisitors :one
SELECT COUNT(DISTINCT visitor_id) as total FROM page_views WHERE visitor_id != '';

-- name: WAClicksByType :many
SELECT click_type, COUNT(*) as clicks
FROM wa_clicks
GROUP BY click_type;
