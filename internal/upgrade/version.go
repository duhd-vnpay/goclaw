package upgrade

// RequiredSchemaVersion is the schema migration version this binary requires.
// Bump this whenever adding a new SQL migration file.
// v3.15.0-beta.174 merge (2026-07-22): prod DB đã apply local numbering tới 106
// (schema_migrations.version=106) nên KHÔNG renumber local. 8 upstream-new
// migrations (theirs 088-095) renumber → 107-114 để chạy sau version hiện hành;
// 21 migration còn lại trùng nội dung (rename-detected) giữ số local.
// v3.15.0-beta.192 merge (2026-08-06): prod ở 114 (CASE A) — local giữ số.
// 1 upstream-new migration (theirs 000096 subagent_tasks_root_agent_scope)
// renumber → 000115. Còn lại trùng nội dung (rename-detected) giữ số local.
const RequiredSchemaVersion uint = 115
