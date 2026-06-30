package upgrade

// RequiredSchemaVersion is the schema migration version this binary requires.
// Bump this whenever adding a new SQL migration file.
// v3.15.0-beta.67 merge: upstream added 3 migrations (085-087: webhook_calls_heartbeat,
// fix_channel_contacts_merged_fk, usage_snapshots_agent_fk). Local 085-103 renumbered
// to 088-106 to come AFTER upstream new migrations.
const RequiredSchemaVersion uint = 106
