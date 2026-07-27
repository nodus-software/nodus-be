INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000001', 'patients:read', 'View patient lists and records'),
  ('00000000-0000-4000-8000-000000000002', 'patients:write', 'Register and update patient records'),
  ('00000000-0000-4000-8000-000000000003', 'users:read', 'View staff accounts and invitations'),
  ('00000000-0000-4000-8000-000000000004', 'users:write', 'Edit, suspend and reactivate staff accounts'),
  ('00000000-0000-4000-8000-000000000005', 'users:invite', 'Send and resend staff invitations'),
  ('00000000-0000-4000-8000-000000000006', 'users:unlock', 'Unlock staff accounts'),
  ('00000000-0000-4000-8000-000000000007', 'roles:read', 'View roles and their permissions'),
  ('00000000-0000-4000-8000-000000000008', 'roles:write', 'Create roles and assign permissions'),
  ('00000000-0000-4000-8000-000000000009', 'access_review:write', 'Review user access'),
  ('00000000-0000-4000-8000-000000000010', 'audit:read', 'View the audit log'),
  ('00000000-0000-4000-8000-000000000011', 'organization:read', 'View organization settings'),
  ('00000000-0000-4000-8000-000000000012', 'organization:write', 'Update organization settings')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;
