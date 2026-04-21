-- name: GetUserFromRefreshToken :one
SELECT users.* FROM refresh_tokens
JOIN users ON users.id = refresh_tokens.user_id
WHERE token = $1
AND expires_at > NOW()
AND revoked_at IS NULL;
