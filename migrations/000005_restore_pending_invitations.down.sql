-- Data repair is intentionally not reversed: an account without a password must
-- not be restored to a normal suspended-account lifecycle.
SELECT 1;
