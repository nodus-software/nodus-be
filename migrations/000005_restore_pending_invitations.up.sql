-- Invited accounts could previously be suspended before password setup. Restore
-- those incomplete accounts to the only valid pre-activation state so admins can
-- see and resend their invitations.
UPDATE users u
SET status = 'invited'
WHERE u.status = 'suspended'
  AND u.password_hash IS NULL
  AND EXISTS (
    SELECT 1 FROM invitations i
    WHERE i.user_id = u.id AND i.tenant_id = u.tenant_id
  );
