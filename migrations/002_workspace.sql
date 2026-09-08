CREATE TABLE owners (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), email text NOT NULL UNIQUE,
 password_hash text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash text PRIMARY KEY, expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE auth_attempts (
 key text PRIMARY KEY, window_start timestamptz NOT NULL DEFAULT now(), attempts integer NOT NULL DEFAULT 0
);
CREATE TABLE environments (
 project_id text NOT NULL REFERENCES projects(id), name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(project_id,name)
);
INSERT INTO environments(project_id,name) SELECT id,'production' FROM projects;
ALTER TABLE services ADD CONSTRAINT service_environment FOREIGN KEY(project_id,environment) REFERENCES environments(project_id,name);
ALTER TABLE services ADD COLUMN desired_state text NOT NULL DEFAULT 'running' CHECK(desired_state IN ('running','stopped'));
CREATE TABLE service_variables (
 service_id text NOT NULL REFERENCES services(id), name text NOT NULL,
 ciphertext bytea NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(service_id,name)
);
ALTER TABLE deployments ADD COLUMN encrypted_env bytea;
CREATE TABLE service_actions (
 id text PRIMARY KEY, service_id text NOT NULL REFERENCES services(id),
 kind text NOT NULL CHECK(kind IN ('start','restart','stop')),
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','done','failed')),
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
