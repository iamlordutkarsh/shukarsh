-- name: InsertProduct :one
INSERT INTO products (url, platform, title, price, original_price, image_url, description, rating)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListProducts :many
SELECT * FROM products ORDER BY added_at DESC;

-- name: GetProduct :one
SELECT * FROM products WHERE id = ?;

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = ?;
