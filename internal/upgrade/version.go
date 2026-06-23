package upgrade

// RequiredSchemaVersion is the schema migration version this binary requires.
// Bump this whenever adding a new SQL migration file.
// v3.15.0-beta.27 merge: upstream added 11 migrations (074-084), renumbered
// to 093-103 to come AFTER local fork migrations (074-092).
const RequiredSchemaVersion uint = 103
