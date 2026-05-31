ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS whitelist_mode VARCHAR(32) NOT NULL DEFAULT 'off';

UPDATE servers
SET whitelist_mode = 'strict_whitelist'
WHERE strict_whitelist = TRUE;
