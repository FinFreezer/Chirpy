-- name: GetUserByRFToken :one
SELECT * FROM users
WHERE id = $1;
