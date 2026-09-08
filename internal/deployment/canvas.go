package deployment

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

type CanvasPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type CanvasResource struct {
	Position          CanvasPosition `json:"position"`
	Key               string         `json:"key"`
	ID                string         `json:"id"`
	Kind              string         `json:"kind"`
	Name              string         `json:"name"`
	Status            string         `json:"status"`
	WorkloadMode      string         `json:"workloadMode,omitempty"`
	SourceType        string         `json:"sourceType,omitempty"`
	Template          string         `json:"template,omitempty"`
	TemplateVersion   string         `json:"templateVersion,omitempty"`
	PublicAddress     string         `json:"publicAddress,omitempty"`
	PrivateAddress    string         `json:"privateAddress,omitempty"`
	ManagedByTemplate bool           `json:"managedByTemplate,omitempty"`
}

type CanvasLink struct {
	ID    string `json:"id"`
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
}

type CanvasGraph struct {
	ProjectID   string           `json:"projectId"`
	Environment string           `json:"environment"`
	Resources   []CanvasResource `json:"resources"`
	Links       []CanvasLink     `json:"links"`
}

type CanvasPositionUpdate struct {
	ResourceKey string `json:"resourceKey"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
}

func deploymentStatus(latest, active, desired string) string {
	if latest != "" && !Terminal(latest) {
		return latest
	}
	if active != "" {
		if desired == "stopped" {
			return "stopped"
		}
		return "active"
	}
	if latest != "" {
		return latest
	}
	return "empty"
}

func defaultCanvasPosition(index int, kind string) CanvasPosition {
	y := 100 + (index/3)*190
	if kind == "database" {
		y += 190
	}
	return CanvasPosition{X: 90 + (index%3)*310, Y: y}
}

func (s *Store) Canvas(ctx context.Context, project, environment string) (CanvasGraph, error) {
	graph := CanvasGraph{ProjectID: project, Environment: environment, Resources: []CanvasResource{}, Links: []CanvasLink{}}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE project_id=$1 AND name=$2)`, project, environment).Scan(&exists); err != nil {
		return graph, err
	}
	if !exists {
		return graph, pgx.ErrNoRows
	}

	positions := map[string]CanvasPosition{}
	rows, err := s.DB.Query(ctx, `SELECT resource_key,x,y FROM canvas_layouts WHERE project_id=$1 AND environment=$2`, project, environment)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var key string
		var position CanvasPosition
		if err = rows.Scan(&key, &position.X, &position.Y); err != nil {
			rows.Close()
			return graph, err
		}
		positions[key] = position
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}
	rows.Close()

	rows, err = s.DB.Query(ctx, `
		SELECT s.id,s.name,s.resource_kind,s.workload_mode,s.template_key,s.template_version,s.host,s.active_id,s.desired_state,
		 COALESCE(source.source_type,''),COALESCE(latest.status,''),COALESCE(latest.port,0)
		FROM services s
		LEFT JOIN service_sources source ON source.service_id=s.id
		LEFT JOIN LATERAL (
		 SELECT status,port FROM deployments WHERE service_id=s.id ORDER BY created_at DESC,id DESC LIMIT 1
		) latest ON true
		WHERE s.project_id=$1 AND s.environment=$2 ORDER BY s.created_at,s.id`, project, environment)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var resource CanvasResource
		var host, active, desired, latest string
		var port int
		if err = rows.Scan(&resource.ID, &resource.Name, &resource.Kind, &resource.WorkloadMode, &resource.Template, &resource.TemplateVersion, &host, &active, &desired, &resource.SourceType, &latest, &port); err != nil {
			rows.Close()
			return graph, err
		}
		resource.Key = "service:" + resource.ID
		resource.Status = deploymentStatus(latest, active, desired)
		if resource.Template != "" {
			resource.SourceType = "template"
		} else if resource.SourceType == "" && latest != "" {
			resource.SourceType = "image"
		} else if resource.SourceType == "" {
			resource.SourceType = "empty"
		}
		if resource.Kind == "database" {
			resource.PrivateAddress = "db-" + resource.ID
			if port > 0 {
				resource.PrivateAddress += fmt.Sprintf(":%d", port)
			}
		} else if resource.WorkloadMode == "web" && host != "" {
			resource.PublicAddress = "http://" + host + ":8088"
			if os.Getenv("PUBLIC_MODE") == "true" {
				resource.PublicAddress = "https://" + host
			}
		}
		resource.Position = defaultCanvasPosition(len(graph.Resources), resource.Kind)
		if saved, ok := positions[resource.Key]; ok {
			resource.Position = saved
		}
		graph.Resources = append(graph.Resources, resource)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}
	rows.Close()

	rows, err = s.DB.Query(ctx, `
		SELECT v.id,v.name,v.managed_by_template,COALESCE(a.service_id,''),COALESCE(a.mount_path,'')
		FROM volumes v LEFT JOIN volume_attachments a ON a.volume_id=v.id
		WHERE v.project_id=$1 AND v.environment=$2 ORDER BY v.created_at,v.id`, project, environment)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var resource CanvasResource
		var serviceID, mountPath string
		if err = rows.Scan(&resource.ID, &resource.Name, &resource.ManagedByTemplate, &serviceID, &mountPath); err != nil {
			rows.Close()
			return graph, err
		}
		resource.Key = "volume:" + resource.ID
		resource.Kind = "volume"
		resource.Status = "available"
		if serviceID != "" {
			resource.Status = "attached"
			graph.Links = append(graph.Links, CanvasLink{ID: "attachment:" + resource.ID, From: resource.Key, To: "service:" + serviceID, Kind: "volume-attachment", Label: mountPath})
		}
		resource.Position = defaultCanvasPosition(len(graph.Resources), resource.Kind)
		if saved, ok := positions[resource.Key]; ok {
			resource.Position = saved
		}
		graph.Resources = append(graph.Resources, resource)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}
	rows.Close()

	rows, err = s.DB.Query(ctx, `SELECT id,name,provider FROM buckets WHERE project_id=$1 AND environment=$2 ORDER BY created_at,id`, project, environment)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var resource CanvasResource
		if err = rows.Scan(&resource.ID, &resource.Name, &resource.Template); err != nil {
			rows.Close()
			return graph, err
		}
		resource.Key = "bucket:" + resource.ID
		resource.Kind = "bucket"
		resource.Status = "available"
		resource.Position = defaultCanvasPosition(len(graph.Resources), resource.Kind)
		if saved, ok := positions[resource.Key]; ok {
			resource.Position = saved
		}
		graph.Resources = append(graph.Resources, resource)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}
	rows.Close()

	rows, err = s.DB.Query(ctx, `
		SELECT r.source_service_id,r.variable_name,r.target_service_id,r.target_variable
		FROM service_references r
		JOIN services source ON source.id=r.source_service_id
		JOIN services target ON target.id=r.target_service_id
		WHERE source.project_id=$1 AND source.environment=$2
		 AND target.project_id=source.project_id AND target.environment=source.environment
		ORDER BY r.created_at,r.source_service_id,r.variable_name`, project, environment)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var source, variable, target, targetVariable string
		if err = rows.Scan(&source, &variable, &target, &targetVariable); err != nil {
			rows.Close()
			return graph, err
		}
		graph.Links = append(graph.Links, CanvasLink{ID: "reference:" + source + ":" + variable, From: "service:" + source, To: "service:" + target, Kind: "variable-reference", Label: variable + " → " + targetVariable})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}
	rows.Close()
	return graph, nil
}

func (s *Store) SaveCanvasLayout(ctx context.Context, project, environment string, updates []CanvasPositionUpdate) error {
	if len(updates) == 0 || len(updates) > 200 {
		return errors.New("provide 1–200 canvas positions")
	}
	graph, err := s.Canvas(ctx, project, environment)
	if err != nil {
		return err
	}
	valid := make(map[string]bool, len(graph.Resources))
	for _, resource := range graph.Resources {
		valid[resource.Key] = true
	}
	seen := map[string]bool{}
	for _, update := range updates {
		if !valid[update.ResourceKey] || seen[update.ResourceKey] || update.X < -100000 || update.X > 100000 || update.Y < -100000 || update.Y > 100000 {
			return errors.New("canvas position contains an unknown, duplicate or invalid resource")
		}
		seen[update.ResourceKey] = true
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, update := range updates {
		if _, err = tx.Exec(ctx, `INSERT INTO canvas_layouts(project_id,environment,resource_key,x,y) VALUES($1,$2,$3,$4,$5)
		 ON CONFLICT(project_id,environment,resource_key) DO UPDATE SET x=EXCLUDED.x,y=EXCLUDED.y,updated_at=now()`, project, environment, update.ResourceKey, update.X, update.Y); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
