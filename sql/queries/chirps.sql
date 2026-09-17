-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid ( ),
    NOW(),
    NOW(),
    $1,
    $2 
)
RETURNING *;

-- name: DeleteChirp :exec
DELETE FROM chirps WHERE id = $1 and user_id = $2;


-- name: GetChirps :many
SELECT * FROM chirps 
ORDER BY 
    CASE WHEN @sort_desc::boolean = false THEN created_at END ASC,
    CASE WHEN @sort_desc::boolean = true THEN created_at END DESC;


-- name: GetChirpById :one
SELECT * FROM chirps WHERE id = $1;

-- name: GetChirpsByAuthor :many
SELECT * FROM chirps WHERE user_id = $1 
ORDER BY 
    CASE WHEN @sort_desc::boolean = false  THEN created_at END ASC,
    CASE WHEN @sort_desc::boolean = true THEN created_at END DESC;