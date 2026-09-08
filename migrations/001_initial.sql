CREATE TABLE IF NOT EXISTS projects (
 id text PRIMARY KEY, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS services (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES projects(id), name text NOT NULL,
 environment text NOT NULL DEFAULT 'production', host text NOT NULL UNIQUE,
 active_id text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id, environment, name)
);
CREATE TABLE IF NOT EXISTS deployments (
 id text PRIMARY KEY, service_id text NOT NULL REFERENCES services(id), image text NOT NULL,
 port integer NOT NULL CHECK(port BETWEEN 1 AND 65535), health_path text NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','pulling','starting','checking','routing','active','failed','superseded')),
 error text NOT NULL DEFAULT '', logs text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_per_service ON deployments(service_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS pending_deployments ON deployments(created_at) WHERE status NOT IN ('active','failed','superseded');
CREATE TABLE IF NOT EXISTS deployment_events (
 id bigserial PRIMARY KEY, deployment_id text NOT NULL REFERENCES deployments(id),
 stage text NOT NULL, message text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS events_by_deployment ON deployment_events(deployment_id,id);
