-- Earlier user-management behavior could reactivate an invited account before
-- password setup. Such an account is not usable and belongs in the invitation
-- lifecycle until its single-use token is accepted.
UPDATE users u
SET status = 'invited'
WHERE u.status = 'active'
  AND u.password_hash IS NULL
  AND EXISTS (
    SELECT 1 FROM invitations i
    WHERE i.user_id = u.id AND i.tenant_id = u.tenant_id
  );
