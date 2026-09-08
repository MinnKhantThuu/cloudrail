CREATE TABLE github_app (id boolean PRIMARY KEY DEFAULT true CHECK(id),app_id text NOT NULL,slug text NOT NULL,private_key bytea NOT NULL,webhook_secret bytea NOT NULL);
CREATE TABLE service_sources (service_id text PRIMARY KEY REFERENCES services(id),config jsonb NOT NULL,updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE source_builds (
 id text PRIMARY KEY,service_id text NOT NULL REFERENCES services(id),commit_sha text NOT NULL,
 config jsonb NOT NULL,status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','building','succeeded','failed','cancelled')),
 image text NOT NULL DEFAULT '',deployment_id text NOT NULL DEFAULT '',logs text NOT NULL DEFAULT '',error text NOT NULL DEFAULT '',
 attempt integer NOT NULL DEFAULT 0,attempt_token text NOT NULL DEFAULT '',deadline timestamptz,next_attempt timestamptz NOT NULL DEFAULT now(),cancel_requested boolean NOT NULL DEFAULT false,
 request_key text,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(service_id,request_key)
);
CREATE TABLE github_deliveries (id text PRIMARY KEY,received_at timestamptz NOT NULL DEFAULT now());
