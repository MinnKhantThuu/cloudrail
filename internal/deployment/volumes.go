package deployment

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Store) CreateVolume(ctx context.Context, project, environment, name string) (Volume, error) {
	if environment == "" {
		environment = "production"
	}
	if !ValidName(name) {
		return Volume{}, errors.New("invalid volume name")
	}
	v := Volume{ID: ID(), ProjectID: project, Environment: environment, Name: name}
	runtimeName := "cloudrail-volume-" + v.ID
	err := s.DB.QueryRow(ctx, `INSERT INTO volumes(id,project_id,environment,name,runtime_name) VALUES($1,$2,$3,$4,$5) RETURNING created_at`, v.ID, project, environment, name, runtimeName).Scan(&v.CreatedAt)
	return v, err
}

func (s *Store) AttachVolume(ctx context.Context, volumeID, serviceID, mountPath string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var runtimeName, project, environment string
	var managed bool
	if err = tx.QueryRow(ctx, `SELECT runtime_name,project_id,environment,managed_by_template FROM volumes WHERE id=$1 FOR UPDATE`, volumeID).Scan(&runtimeName, &project, &environment, &managed); err != nil {
		return err
	}
	if managed {
		return errors.New("template-managed volumes cannot be reattached")
	}
	var attached bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM volume_attachments WHERE volume_id=$1)`, volumeID).Scan(&attached); err != nil {
		return err
	}
	if attached {
		return errors.New("volume is already attached to an application")
	}
	var raw []byte
	var serviceProject, serviceEnvironment, resourceKind, templateKey, desired, active string
	if err = tx.QueryRow(ctx, `SELECT settings,project_id,environment,resource_kind,template_key,desired_state,active_id FROM services WHERE id=$1 FOR UPDATE`, serviceID).Scan(&raw, &serviceProject, &serviceEnvironment, &resourceKind, &templateKey, &desired, &active); err != nil {
		return err
	}
	if project != serviceProject || environment != serviceEnvironment || resourceKind != "service" || templateKey != "" {
		return errors.New("choose an application in the same project and environment")
	}
	if active != "" && desired != "stopped" {
		return errors.New("stop the application before attaching a volume")
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM volume_attachments WHERE service_id=$1)`, serviceID).Scan(&attached); err != nil {
		return err
	}
	if attached {
		return errors.New("application already has a volume")
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE service_id=$1 AND status NOT IN ('active','failed','superseded')) OR EXISTS(SELECT 1 FROM service_actions WHERE service_id=$1 AND status IN ('queued','running')) OR EXISTS(SELECT 1 FROM cron_runs WHERE service_id=$1 AND status IN ('queued','running'))`, serviceID).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return errors.New("application has unfinished work")
	}
	var settings Settings
	if err = json.Unmarshal(raw, &settings); err != nil {
		return err
	}
	settings.MountPath = mountPath
	settings.VolumeName = runtimeName
	if err = settings.Validate(); err != nil {
		return err
	}
	updated, _ := json.Marshal(settings)
	if _, err = tx.Exec(ctx, `INSERT INTO volume_attachments(volume_id,service_id,mount_path) VALUES($1,$2,$3)`, volumeID, serviceID, mountPath); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE services SET settings=$2 WHERE id=$1`, serviceID, updated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DetachVolume(ctx context.Context, volumeID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var serviceID string
	var managed bool
	if err = tx.QueryRow(ctx, `SELECT a.service_id,v.managed_by_template FROM volumes v JOIN volume_attachments a ON a.volume_id=v.id WHERE v.id=$1 FOR UPDATE OF v,a`, volumeID).Scan(&serviceID, &managed); err != nil {
		return err
	}
	if managed {
		return errors.New("template-managed volumes cannot be detached")
	}
	var raw []byte
	var desired, active string
	if err = tx.QueryRow(ctx, `SELECT settings,desired_state,active_id FROM services WHERE id=$1 FOR UPDATE`, serviceID).Scan(&raw, &desired, &active); err != nil {
		return err
	}
	if active != "" && desired != "stopped" {
		return errors.New("stop the application before detaching its volume")
	}
	var settings Settings
	if err = json.Unmarshal(raw, &settings); err != nil {
		return err
	}
	settings.MountPath, settings.VolumeName = "", ""
	updated, _ := json.Marshal(settings)
	if _, err = tx.Exec(ctx, `DELETE FROM volume_attachments WHERE volume_id=$1`, volumeID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE services SET settings=$2 WHERE id=$1`, serviceID, updated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
