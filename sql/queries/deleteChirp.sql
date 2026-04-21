-- name: DeleteChirp :exec
DELETE FROM chirps
WHERE chirps.id = $1;
