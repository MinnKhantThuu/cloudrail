ALTER TABLE services ADD COLUMN resource_kind text NOT NULL DEFAULT 'service'
 CHECK(resource_kind IN ('service','database'));
ALTER TABLE services ADD COLUMN workload_mode text NOT NULL DEFAULT 'web'
 CHECK(workload_mode IN ('web','worker','cron'));
ALTER TABLE services ADD COLUMN template_key text NOT NULL DEFAULT '';
UPDATE services SET resource_kind='database',template_key='postgres'
 WHERE settings->>'kind'='postgres';

ALTER TABLE service_sources ADD COLUMN source_type text NOT NULL DEFAULT 'github'
 CHECK(source_type IN ('github','image','empty','function','template'));

CREATE TABLE volumes (
 id text PRIMARY KEY,
 project_id text NOT NULL,
 environment text NOT NULL,
 name text NOT NULL,
 driver text NOT NULL DEFAULT 'local' CHECK(driver IN ('local')),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(project_id,environment) REFERENCES environments(project_id,name),
 UNIQUE(project_id,environment,name)
);

CREATE TABLE volume_attachments (
 volume_id text PRIMARY KEY REFERENCES volumes(id),
 service_id text NOT NULL UNIQUE REFERENCES services(id),
 mount_path text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO volumes(id,project_id,environment,name)
 SELECT md5('volume:'||id),project_id,environment,settings->>'volumeName'
 FROM services WHERE COALESCE(settings->>'volumeName','')<>'';
INSERT INTO volume_attachments(volume_id,service_id,mount_path)
 SELECT md5('volume:'||id),id,settings->>'mountPath'
 FROM services WHERE COALESCE(settings->>'volumeName','')<>'';

CREATE TABLE buckets (
 id text PRIMARY KEY,
 project_id text NOT NULL,
 environment text NOT NULL,
 name text NOT NULL,
 provider text NOT NULL DEFAULT 's3' CHECK(provider IN ('s3')),
 endpoint text NOT NULL,
 region text NOT NULL DEFAULT '',
 credentials bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(project_id,environment) REFERENCES environments(project_id,name),
 UNIQUE(project_id,environment,name)
);

CREATE TABLE service_references (
 source_service_id text NOT NULL REFERENCES services(id),
 variable_name text NOT NULL,
 target_service_id text NOT NULL REFERENCES services(id),
 target_variable text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(source_service_id,variable_name),
 CHECK(source_service_id<>target_service_id)
);

CREATE TABLE canvas_layouts (
 project_id text NOT NULL,
 environment text NOT NULL,
 resource_key text NOT NULL CHECK(length(resource_key) BETWEEN 3 AND 160),
 x integer NOT NULL CHECK(x BETWEEN -100000 AND 100000),
 y integer NOT NULL CHECK(y BETWEEN -100000 AND 100000),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(project_id,environment,resource_key),
 FOREIGN KEY(project_id,environment) REFERENCES environments(project_id,name)
);

CREATE INDEX service_canvas_scope ON services(project_id,environment,created_at);
CREATE INDEX volume_canvas_scope ON volumes(project_id,environment,created_at);
CREATE INDEX bucket_canvas_scope ON buckets(project_id,environment,created_at);
CREATE INDEX reference_targets ON service_references(target_service_id);
