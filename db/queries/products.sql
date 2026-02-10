-- name: InsertProduct :one
INSERT INTO products (url, platform, title, price, original_price, image_url, description, rating, category, images, long_description)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListProducts :many
SELECT * FROM products ORDER BY added_at DESC;

-- name: GetProduct :one
SELECT * FROM products WHERE id = ?;

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = ?;

-- name: UpdateCategory :exec
UPDATE products SET category = ? WHERE id = ?;

-- name: ListCategories :many
SELECT DISTINCT category FROM products WHERE category != '' ORDER BY category;

-- name: SearchProducts :many
SELECT * FROM products WHERE title LIKE ? OR description LIKE ? OR category LIKE ? ORDER BY added_at DESC;

-- name: UpdateProductImages :exec
UPDATE products SET images = ?, long_description = ? WHERE id = ?;

-- name: ListProductsByCategory :many
SELECT * FROM products WHERE category = ? ORDER BY added_at DESC;

-- name: UpdateProduct :exec
UPDATE products SET
  title = ?,
  price = ?,
  original_price = ?,
  image_url = ?,
  description = ?,
  rating = ?,
  category = ?,
  images = ?,
  long_description = ?,
  url = ?,
  platform = ?
WHERE id = ?;
