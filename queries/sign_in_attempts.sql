-- name: LockSignInFingerprint :exec
SELECT pg_advisory_xact_lock(@key::bigint);

-- name: CountSignInAttemptsSince :one
SELECT count(*)
FROM sign_in_attempts
WHERE kind = @kind
  AND fingerprint = @fingerprint
  AND created_at >= @since;

-- name: InsertSignInAttempt :exec
INSERT INTO sign_in_attempts (kind, fingerprint, created_at)
VALUES (@kind, @fingerprint, @created_at);

-- name: DeleteSignInAttemptsBefore :execrows
DELETE FROM sign_in_attempts
WHERE created_at < @before;
