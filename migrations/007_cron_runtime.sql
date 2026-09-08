ALTER TABLE services ADD COLUMN cron_schedule text NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN cron_next_run timestamptz;

CREATE TABLE cron_runs (
 id text PRIMARY KEY,
 service_id text NOT NULL REFERENCES services(id),
 deployment_id text NOT NULL REFERENCES deployments(id),
 scheduled_for timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','succeeded','failed')),
 exit_code integer,
 logs text NOT NULL DEFAULT '',
 error text NOT NULL DEFAULT '',
 attempt integer NOT NULL DEFAULT 0,
 attempt_token text NOT NULL DEFAULT '',
 next_attempt timestamptz NOT NULL DEFAULT now(),
 deadline timestamptz,
 started_at timestamptz,
 finished_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(service_id,scheduled_for)
);

CREATE UNIQUE INDEX cron_one_unfinished_run_per_service
 ON cron_runs(service_id) WHERE status IN ('queued','running');
CREATE INDEX cron_run_history ON cron_runs(service_id,scheduled_for DESC);
CREATE INDEX cron_due_services ON services(cron_next_run)
 WHERE workload_mode='cron' AND desired_state='running' AND cron_schedule<>'';
