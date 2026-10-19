-- +goose Up
-- Deleting an admin used to leave their accepted invite open with accepted_by nulled by the foreign key. Close those, as deletion now does.
UPDATE admin_invites SET revoked_at = now()
WHERE accepted_at IS NOT NULL AND accepted_by IS NULL AND revoked_at IS NULL;

-- +goose Down
SELECT 1;
