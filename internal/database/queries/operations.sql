-- name: CreateOperation :one
INSERT INTO operations (
    idempotency_key,
    request_hash
) VALUES (
    $1,
    $2
)
RETURNING *;

-- name: GetOperationByIdempotencyKey :one
SELECT * FROM operations
WHERE idempotency_key = $1;

-- name: UpdateOperationStatus :one
UPDATE operations
SET 
    status = $2,
    result = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;


