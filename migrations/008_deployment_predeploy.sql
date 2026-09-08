ALTER TABLE deployments DROP CONSTRAINT IF EXISTS deployments_status_check;
ALTER TABLE deployments ADD CONSTRAINT deployments_status_check
 CHECK(status IN ('queued','pulling','predeploy','starting','checking','routing','active','failed','superseded'));
