ALTER TABLE services ADD COLUMN template_version text NOT NULL DEFAULT ''
 CHECK(length(template_version) <= 40);

UPDATE services SET template_version='17.6'
 WHERE template_key='postgres' AND template_version='';
