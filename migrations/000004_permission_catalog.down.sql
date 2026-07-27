DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions WHERE code IN (
    'patients:read', 'patients:write', 'users:read', 'users:write',
    'users:invite', 'users:unlock', 'roles:read', 'roles:write',
    'access_review:write', 'audit:read', 'organization:read', 'organization:write'
  )
);

DELETE FROM permissions WHERE code IN (
  'patients:read', 'patients:write', 'users:read', 'users:write',
  'users:invite', 'users:unlock', 'roles:read', 'roles:write',
  'access_review:write', 'audit:read', 'organization:read', 'organization:write'
);
