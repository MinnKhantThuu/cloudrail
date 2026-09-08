CREATE TABLE nodes (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), serial text NOT NULL, csr_hash text NOT NULL,
 certificate text NOT NULL, revoked boolean NOT NULL DEFAULT false,
 last_seen timestamptz NOT NULL DEFAULT now(), enrolled_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE deployments ADD COLUMN request_key text;
ALTER TABLE deployments ADD COLUMN request_hash text;
CREATE UNIQUE INDEX deployment_request_key ON deployments(service_id,request_key) WHERE request_key IS NOT NULL;
ALTER TABLE deployments ADD COLUMN attempt integer NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN attempt_token text NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN deadline timestamptz;
ALTER TABLE deployments ADD COLUMN next_attempt timestamptz NOT NULL DEFAULT now();
ALTER TABLE deployments ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false;
ALTER TABLE service_actions ADD COLUMN attempt integer NOT NULL DEFAULT 0;
ALTER TABLE service_actions ADD COLUMN attempt_token text NOT NULL DEFAULT '';
ALTER TABLE service_actions ADD COLUMN deadline timestamptz;
ALTER TABLE service_actions ADD COLUMN next_attempt timestamptz NOT NULL DEFAULT now();
