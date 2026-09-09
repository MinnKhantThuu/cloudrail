ALTER TABLE service_variables
 ADD COLUMN kind text NOT NULL DEFAULT 'secret'
 CHECK(kind IN ('secret','plain','reference'));

UPDATE service_variables variable
SET kind='reference'
WHERE EXISTS (
 SELECT 1 FROM service_references reference
 WHERE reference.source_service_id=variable.service_id
   AND reference.variable_name=variable.name
);
