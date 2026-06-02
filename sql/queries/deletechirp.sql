-- name: DeleteChirp :exec
DELETE from chirps
WHERE id = $1 AND user_id = $2;
