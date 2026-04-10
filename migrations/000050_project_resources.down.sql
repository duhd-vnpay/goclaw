-- migrations/000050_project_resources.down.sql

DROP VIEW IF EXISTS project_mcp_overrides_compat;
DROP TABLE IF EXISTS ardenn_domain_resource_templates;
DROP TABLE IF EXISTS project_resources;
