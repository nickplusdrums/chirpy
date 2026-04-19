-- name: GetUserByEmail :one
SELECT * FROM users
WHERE $1 = email;
