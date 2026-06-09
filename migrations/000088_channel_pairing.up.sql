-- Migration 000060: Channel Pairing via Email OTP
-- Adds pairing_verifications table and identity columns to channel_contacts/paired_devices.

CREATE TABLE IF NOT EXISTS pairing_verifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES org_users(id),
    email           VARCHAR(255) NOT NULL,
    code            VARCHAR(6) NOT NULL,
    channel_type    VARCHAR(50) NOT NULL,
    sender_id       VARCHAR(255) NOT NULL,
    chat_id         VARCHAR(255),
    attempts        INT NOT NULL DEFAULT 0,
    expires_at      TIMESTAMPTZ NOT NULL,
    verified_at     TIMESTAMPTZ,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Rate-limit index: find pending verifications per sender quickly
CREATE INDEX IF NOT EXISTS idx_pairing_rate
    ON pairing_verifications(sender_id, created_at) WHERE verified_at IS NULL;

-- Tenant-scoped lookup
CREATE INDEX IF NOT EXISTS idx_pairing_verifications_tenant
    ON pairing_verifications(tenant_id);

-- Add identity columns to channel_contacts
ALTER TABLE channel_contacts ADD COLUMN IF NOT EXISTS email VARCHAR(255);
ALTER TABLE channel_contacts ADD COLUMN IF NOT EXISTS verified_user_id UUID REFERENCES org_users(id);

-- Add identity columns to paired_devices
ALTER TABLE paired_devices ADD COLUMN IF NOT EXISTS verified_user_id UUID REFERENCES org_users(id);
ALTER TABLE paired_devices ADD COLUMN IF NOT EXISTS email VARCHAR(255);
