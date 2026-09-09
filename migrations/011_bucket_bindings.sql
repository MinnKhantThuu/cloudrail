ALTER TABLE buckets ADD COLUMN bucket_name text NOT NULL DEFAULT '';
ALTER TABLE buckets ADD COLUMN force_path_style boolean NOT NULL DEFAULT true;
ALTER TABLE buckets ADD COLUMN credential_version integer NOT NULL DEFAULT 1;
ALTER TABLE buckets ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

UPDATE buckets SET bucket_name=name WHERE bucket_name='';

ALTER TABLE buckets ADD CONSTRAINT bucket_name_length CHECK(length(bucket_name) BETWEEN 3 AND 63);
ALTER TABLE buckets ADD CONSTRAINT bucket_endpoint_length CHECK(length(endpoint) BETWEEN 8 AND 2048);
ALTER TABLE buckets ADD CONSTRAINT bucket_region_length CHECK(length(region) <= 100);
ALTER TABLE buckets ADD CONSTRAINT bucket_credential_version_positive CHECK(credential_version > 0);

CREATE TABLE bucket_bindings (
 bucket_id text NOT NULL REFERENCES buckets(id) ON DELETE CASCADE,
 service_id text NOT NULL REFERENCES services(id) ON DELETE CASCADE,
 variable_prefix text NOT NULL CHECK(variable_prefix ~ '^[A-Z][A-Z0-9_]{0,31}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(bucket_id,service_id),
 UNIQUE(service_id,variable_prefix)
);

CREATE INDEX bucket_binding_service ON bucket_bindings(service_id);
