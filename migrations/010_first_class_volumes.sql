ALTER TABLE volumes ADD COLUMN runtime_name text NOT NULL DEFAULT '';
ALTER TABLE volumes ADD COLUMN managed_by_template boolean NOT NULL DEFAULT false;

UPDATE volumes SET runtime_name=name WHERE runtime_name='';
UPDATE volumes v SET managed_by_template=true
 FROM volume_attachments a JOIN services s ON s.id=a.service_id
 WHERE a.volume_id=v.id AND s.template_key<>'';

ALTER TABLE volumes ADD CONSTRAINT volume_runtime_name_length
 CHECK(length(runtime_name) BETWEEN 1 AND 160);
