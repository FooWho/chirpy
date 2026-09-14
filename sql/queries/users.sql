-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid ( ),
    NOW(),
    NOW(),
    $1,
    $2 
)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET email = $1, hashed_password = $2
WHERE id = $3
RETURNING *;

-- name: ResetUsers :exec
TRUNCATE TABLE users CASCADE;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserById :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByRefreshToken :one
SELECT users.* 
FROM users
JOIN refresh_tokens ON users.id = refresh_tokens.user_id
WHERE refresh_tokens.token = $1 
  AND refresh_tokens.revoked_at IS NULL 
  AND refresh_tokens.expires_at > NOW();

-- name: SetUserToRed :one
UPDATE users SET is_chirpy_red = TRUE WHERE id = $1
RETURNING *;