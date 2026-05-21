-- Migration 000060 down: Channel Pairing via Email OTP

ALTER TABLE paired_devices DROP COLUMN IF EXISTS email;
ALTER TABLE paired_devices DROP COLUMN IF EXISTS verified_user_id;
ALTER TABLE channel_contacts DROP COLUMN IF EXISTS verified_user_id;
ALTER TABLE channel_contacts DROP COLUMN IF EXISTS email;
DROP INDEX IF EXISTS idx_pairing_verifications_tenant;
DROP INDEX IF EXISTS idx_pairing_rate;
DROP TABLE IF EXISTS pairing_verifications;
