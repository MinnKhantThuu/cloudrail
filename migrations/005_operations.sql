ALTER TABLE services ADD COLUMN settings jsonb NOT NULL DEFAULT '{"kind":"http","memoryMB":256,"cpuMillis":1000,"mountPath":"","volumeName":"","network":""}';
ALTER TABLE deployments ADD COLUMN settings jsonb NOT NULL DEFAULT '{"kind":"http","memoryMB":256,"cpuMillis":1000,"mountPath":"","volumeName":"","network":""}';
ALTER TABLE nodes ADD COLUMN metrics jsonb NOT NULL DEFAULT '{}';
CREATE TABLE backups(id text PRIMARY KEY,service_id text NOT NULL REFERENCES services(id),size_bytes bigint NOT NULL,checksum text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE service_actions DROP CONSTRAINT service_actions_kind_check;
ALTER TABLE service_actions ADD CONSTRAINT service_actions_kind_check CHECK(kind IN ('start','stop','restart','backup','restore'));
ALTER TABLE service_actions ADD COLUMN backup_id text NOT NULL DEFAULT '';
