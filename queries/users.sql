-- name: CreateUser :one
INSERT INTO users DEFAULT VALUES
RETURNING id;

-- name: GetUserStatus :one
SELECT status FROM users WHERE id = @id;

-- name: SetUserStatus :execrows
UPDATE users SET status = @status, updated_at = now() WHERE id = @id;

-- name: GetIdentityBySupertokensID :one
SELECT i.id, i.user_id, i.kind, i.subject, u.status
FROM user_identities i
JOIN users u ON u.id = i.user_id
WHERE i.supertokens_user_id = @supertokens_user_id;

-- name: GetIdentityBySubject :one
SELECT i.id, i.user_id, i.supertokens_user_id, u.status
FROM user_identities i
JOIN users u ON u.id = i.user_id
WHERE i.kind = @kind AND i.subject = @subject;

-- name: CreateIdentity :exec
INSERT INTO user_identities (user_id, kind, subject, email, supertokens_user_id, verified_at)
VALUES (@user_id, @kind, @subject, @email, @supertokens_user_id, @verified_at);

-- name: UpdateIdentityEmail :exec
UPDATE user_identities SET email = @email WHERE id = @id AND email IS DISTINCT FROM @email;

-- name: ListSupertokensUserIDs :many
SELECT supertokens_user_id FROM user_identities WHERE user_id = @user_id;

-- name: AddRole :exec
INSERT INTO user_roles (user_id, role, granted_by)
VALUES (@user_id, @role, @granted_by)
ON CONFLICT (user_id, role) DO NOTHING;

-- name: RemoveRole :execrows
DELETE FROM user_roles WHERE user_id = @user_id AND role = @role;

-- name: ListRoles :many
SELECT role FROM user_roles WHERE user_id = @user_id ORDER BY role;

-- name: CreateAdminInvite :one
INSERT INTO admin_invites (email, invited_by)
VALUES (@email, @invited_by)
RETURNING id, email, invited_at;

-- name: GetOpenAdminInviteForUpdate :one
SELECT id, accepted_by
FROM admin_invites
WHERE email = @email AND revoked_at IS NULL
FOR UPDATE;

-- name: AcceptAdminInvite :exec
UPDATE admin_invites SET accepted_by = @accepted_by, accepted_at = now() WHERE id = @id;

-- name: RevokeAdminInvite :one
UPDATE admin_invites SET revoked_at = now()
WHERE id = @id AND revoked_at IS NULL
RETURNING accepted_by;

-- name: ListAdminInvites :many
SELECT id, email, invited_by, invited_at, accepted_by, accepted_at, revoked_at
FROM admin_invites
ORDER BY invited_at DESC
LIMIT 200;

-- name: InsertAdminAudit :exec
INSERT INTO admin_audit_log (actor_id, action, target)
VALUES (@actor_id, @action, @target);
