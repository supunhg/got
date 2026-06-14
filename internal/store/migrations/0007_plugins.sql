-- 0007_plugins.sql (applied at v0.6)
-- Plugin Runtime v2 schema: tracks installed plugins, their manifests, and state.

CREATE TABLE IF NOT EXISTS plugins (
  id              TEXT PRIMARY KEY,                -- ULID
  name            TEXT NOT NULL UNIQUE,            -- plugin name from manifest
  version         TEXT NOT NULL,                   -- semver from manifest
  description     TEXT NOT NULL DEFAULT '',
  path            TEXT NOT NULL,                   -- absolute path to plugin directory
  enabled         INTEGER NOT NULL DEFAULT 1,      -- 1 = enabled, 0 = disabled
  manifest_json   TEXT NOT NULL DEFAULT '{}',      -- full manifest content as JSON
  installed_at    INTEGER NOT NULL                 -- unix ms
);

CREATE INDEX IF NOT EXISTS plugins_name_idx ON plugins(name);
CREATE INDEX IF NOT EXISTS plugins_enabled_idx ON plugins(enabled);
