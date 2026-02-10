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
  platform = ?,
  is_new = ?,
  is_bestseller = ?
WHERE id = ?;

-- name: ListNewArrivals :many
SELECT * FROM products WHERE is_new = 1 ORDER BY added_at DESC;

-- name: ListBestSellers :many
SELECT * FROM products WHERE is_bestseller = 1 ORDER BY added_at DESC;

-- name: UpdateProductTags :exec
UPDATE products SET is_new = ?, is_bestseller = ? WHERE id = ?;
