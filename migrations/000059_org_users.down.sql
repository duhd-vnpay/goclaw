-- migrations/000059_org_users.down.sql
DROP INDEX IF EXISTS idx_tenant_users_keycloak_id;
ALTER TABLE tenant_users DROP COLUMN IF EXISTS keycloak_id;
DROP INDEX IF EXISTS idx_org_users_email;
DROP INDEX IF EXISTS idx_org_users_tenant_id;
DROP TABLE IF EXISTS org_users;
