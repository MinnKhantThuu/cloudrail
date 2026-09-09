package api

import (
	"bytes"
	"cloudrail/internal/auth"
	"cloudrail/internal/deployment"
	"cloudrail/internal/secrets"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCanvasGraphAndPersistedLayout(t *testing.T) {
	if os.Getenv("CLOUDRAIL_INTEGRATION") != "1" {
		t.Skip("requires PostgreSQL integration environment")
	}
	ctx := context.Background()
	base := os.Getenv("DATABASE_URL")
	db, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := "canvas_verify_" + deployment.ID()
	if _, err = db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	store, err := deployment.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	store.Cipher, err = secrets.New(os.Getenv("CLOUDRAIL_ENCRYPTION_KEY"))
	if err != nil {
		t.Fatal(err)
	}

	project, err := store.CreateProject(ctx, "canvas contract")
	if err != nil {
		t.Fatal(err)
	}
	application, err := store.CreateService(ctx, project.ID, "api")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveSettings(ctx, application.ID, 256, 1000, "/data"); err != nil {
		t.Fatal(err)
	}
	database, err := store.CreateDatabase(ctx, project.ID, "postgres", "production")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.BindDatabase(ctx, database.ID, application.ID, "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}

	sessions := &auth.Auth{DB: store.DB}
	if err = sessions.Setup(ctx, "canvas@example.com", "canvas-test-password"); err != nil {
		t.Fatal(err)
	}
	token, err := sessions.Login(ctx, "canvas@example.com", "canvas-test-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := (&API{Sessions: sessions, Store: store}).Handler(t.TempDir())
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.AddCookie(&http.Cookie{Name: "cloudrail_session", Value: token})
		if method != http.MethodGet {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Cloudrail-Request", "1")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := request(http.MethodPut, "/api/services/"+application.ID+"/runtime", deployment.RuntimeSettings{StartCommand: "node server.js", PreDeployCommand: "node migrate.js", PreDeployTimeoutSeconds: 120, RestartPolicy: "on-failure", RestartMaxRetries: 7})
	if w.Code != http.StatusOK {
		t.Fatalf("runtime settings response: %d %s", w.Code, w.Body.String())
	}
	var runtimeSettings deployment.Settings
	if err = json.Unmarshal(w.Body.Bytes(), &runtimeSettings); err != nil || runtimeSettings.StartCommand != "node server.js" || runtimeSettings.PreDeployTimeoutSeconds != 120 || runtimeSettings.RestartMaxRetries != 7 {
		t.Fatalf("runtime settings contract is wrong: %#v %v", runtimeSettings, err)
	}
	w = request(http.MethodPut, "/api/services/"+application.ID+"/runtime", deployment.RuntimeSettings{PreDeployTimeoutSeconds: 120, RestartPolicy: "sometimes"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid runtime settings accepted: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodGet, "/api/templates", nil)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"key":"redis"`)) || bytes.Contains(w.Body.Bytes(), []byte("REDIS_PASSWORD")) {
		t.Fatalf("safe template catalog response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/databases", map[string]string{"name": "cache", "environment": "production", "template": "redis"})
	if w.Code != http.StatusCreated {
		t.Fatalf("redis create response: %d %s", w.Code, w.Body.String())
	}
	var redis deployment.Service
	if err = json.Unmarshal(w.Body.Bytes(), &redis); err != nil || redis.Template != "redis" || redis.TemplateVersion != "8.2.2" || redis.Settings.Kind != "redis" || redis.Settings.MountPath != "/data" {
		t.Fatalf("redis service contract is wrong: %#v %v", redis, err)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("redis://default:")) {
		t.Fatalf("redis credentials leaked in create response: %s", w.Body.String())
	}
	w = request(http.MethodGet, "/api/services/"+redis.ID+"/variables", nil)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("REDIS_URL")) || bytes.Contains(w.Body.Bytes(), []byte("redis://")) {
		t.Fatalf("redis variable names contract is wrong: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPost, "/api/services/"+redis.ID+"/bindings", map[string]string{"targetServiceId": application.ID, "variableName": "REDIS_URL"})
	if w.Code != http.StatusOK {
		t.Fatalf("redis binding response: %d %s", w.Code, w.Body.String())
	}
	var redisDeployment string
	if err = store.DB.QueryRow(ctx, `SELECT id FROM deployments WHERE service_id=$1`, redis.ID).Scan(&redisDeployment); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE deployments SET status='active' WHERE id=$1`, redisDeployment); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE services SET active_id=$2,desired_state='stopped' WHERE id=$1`, redis.ID, redisDeployment); err != nil {
		t.Fatal(err)
	}
	w = request(http.MethodPost, "/api/services/"+redis.ID+"/actions", map[string]string{"kind": "backup"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("stopped Redis backup was rejected: %d %s", w.Code, w.Body.String())
	}
	if _, err = store.DB.Exec(ctx, `UPDATE service_actions SET status='done' WHERE service_id=$1`, redis.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE services SET desired_state='running' WHERE id=$1`, redis.ID); err != nil {
		t.Fatal(err)
	}
	w = request(http.MethodPost, "/api/services/"+redis.ID+"/actions", map[string]string{"kind": "backup"})
	if w.Code != http.StatusConflict {
		t.Fatalf("running Redis backup guard is wrong: %d %s", w.Code, w.Body.String())
	}
	if _, actionErr := store.EnqueueAction(ctx, redis.ID, "backup"); actionErr == nil || !strings.Contains(actionErr.Error(), "stopped Redis") {
		t.Fatalf("running Redis store guard is wrong: %v", actionErr)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE services SET desired_state='stopped' WHERE id=$1`, redis.ID); err != nil {
		t.Fatal(err)
	}

	canvasPath := "/api/projects/" + project.ID + "/environments/production/canvas"
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/resources", deployment.ComputeSpec{Name: "queue", Environment: "production", WorkloadMode: "worker", SourceType: "github"})
	if w.Code != http.StatusCreated {
		t.Fatalf("worker create response: %d %s", w.Code, w.Body.String())
	}
	var worker deployment.Service
	if err = json.Unmarshal(w.Body.Bytes(), &worker); err != nil {
		t.Fatal(err)
	}
	if worker.ResourceKind != "service" || worker.WorkloadMode != "worker" || worker.URL != "" {
		t.Fatalf("worker contract is wrong: %#v", worker)
	}
	var workerSource string
	if err = store.DB.QueryRow(ctx, `SELECT source_type FROM service_sources WHERE service_id=$1`, worker.ID).Scan(&workerSource); err != nil || workerSource != "github" {
		t.Fatalf("worker source was not recorded: %q %v", workerSource, err)
	}
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/resources", deployment.ComputeSpec{Name: "cleanup", Environment: "production", WorkloadMode: "cron", SourceType: "image"})
	if w.Code != http.StatusCreated {
		t.Fatalf("cron create response: %d %s", w.Code, w.Body.String())
	}
	var cronService deployment.Service
	if err = json.Unmarshal(w.Body.Bytes(), &cronService); err != nil {
		t.Fatal(err)
	}
	w = request(http.MethodPut, "/api/services/"+cronService.ID+"/cron", map[string]string{"schedule": "@hourly"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("descriptor cron schedule accepted: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPut, "/api/services/"+cronService.ID+"/cron", map[string]string{"schedule": "*/15 * * * *"})
	if w.Code != http.StatusOK {
		t.Fatalf("cron schedule response: %d %s", w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &cronService); err != nil || cronService.CronSchedule != "*/15 * * * *" || cronService.CronNextRun == nil {
		t.Fatalf("cron schedule contract is wrong: %#v %v", cronService, err)
	}
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/resources", deployment.ComputeSpec{Name: "bad", Environment: "production", WorkloadMode: "daemon", SourceType: "image"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid workload accepted: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/volumes", map[string]string{"name": "worker data", "environment": "production"})
	if w.Code != http.StatusCreated {
		t.Fatalf("volume create response: %d %s", w.Code, w.Body.String())
	}
	var volume deployment.Volume
	if err = json.Unmarshal(w.Body.Bytes(), &volume); err != nil || volume.Name != "worker data" || volume.ManagedByTemplate {
		t.Fatalf("volume contract is wrong: %#v %v", volume, err)
	}
	w = request(http.MethodPut, "/api/volumes/"+volume.ID+"/attachment", map[string]string{"serviceId": worker.ID, "mountPath": "/cache"})
	if w.Code != http.StatusOK {
		t.Fatalf("volume attach response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPut, "/api/volumes/"+volume.ID+"/attachment", map[string]string{"serviceId": cronService.ID, "mountPath": "/cache"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("single-writer guard accepted a second service: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodDelete, "/api/volumes/"+deployment.VolumeID(database.ID)+"/attachment", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("template volume detach was accepted: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodDelete, "/api/volumes/"+volume.ID+"/attachment", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("volume detach response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPut, "/api/volumes/"+volume.ID+"/attachment", map[string]string{"serviceId": worker.ID, "mountPath": "/etc"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsafe mount path accepted: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPut, "/api/volumes/"+volume.ID+"/attachment", map[string]string{"serviceId": worker.ID, "mountPath": "/cache"})
	if w.Code != http.StatusOK {
		t.Fatalf("volume reattach response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/buckets", deployment.BucketSpec{
		Name: "invalid", Environment: "production", Endpoint: "file:///tmp/bucket", Region: "us-east-1", BucketName: "invalid-bucket",
		AccessKeyID: "initial-access-key", SecretAccessKey: "initial-secret-key", ForcePathStyle: true,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsafe bucket endpoint was accepted: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodPost, "/api/projects/"+project.ID+"/buckets", deployment.BucketSpec{
		Name: "uploads", Environment: "production", Endpoint: "http://minio:9000", Region: "us-east-1", BucketName: "app-uploads",
		AccessKeyID: "initial-access-key", SecretAccessKey: "initial-secret-key", ForcePathStyle: true,
	})
	if w.Code != http.StatusCreated || bytes.Contains(w.Body.Bytes(), []byte("initial-access-key")) || bytes.Contains(w.Body.Bytes(), []byte("initial-secret-key")) {
		t.Fatalf("safe bucket create response is wrong: %d %s", w.Code, w.Body.String())
	}
	var bucket deployment.Bucket
	if err = json.Unmarshal(w.Body.Bytes(), &bucket); err != nil || bucket.BucketName != "app-uploads" || bucket.Endpoint != "http://minio:9000" || bucket.CredentialVersion != 1 || !bucket.ForcePathStyle {
		t.Fatalf("bucket contract is wrong: %#v %v", bucket, err)
	}
	w = request(http.MethodPut, "/api/buckets/"+bucket.ID+"/bindings/"+application.ID, map[string]string{"variablePrefix": "uploads"})
	if w.Code != http.StatusOK {
		t.Fatalf("bucket binding response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodGet, "/api/services/"+application.ID+"/variables", nil)
	for _, name := range []string{"UPLOADS_ACCESS_KEY_ID", "UPLOADS_BUCKET", "UPLOADS_ENDPOINT", "UPLOADS_FORCE_PATH_STYLE", "UPLOADS_REGION", "UPLOADS_SECRET_ACCESS_KEY"} {
		if !bytes.Contains(w.Body.Bytes(), []byte(name)) {
			t.Fatalf("bucket variable %s missing: %s", name, w.Body.String())
		}
	}
	if variableErr := store.SetVariable(ctx, application.ID, "UPLOADS_SECRET_ACCESS_KEY", "manual-secret"); variableErr == nil || !strings.Contains(variableErr.Error(), "managed") {
		t.Fatalf("managed bucket variable overwrite was accepted: %v", variableErr)
	}
	if variableErr := store.DeleteVariable(ctx, application.ID, "UPLOADS_SECRET_ACCESS_KEY"); variableErr == nil || !strings.Contains(variableErr.Error(), "disconnect") {
		t.Fatalf("managed bucket variable deletion was accepted: %v", variableErr)
	}
	w = request(http.MethodPut, "/api/buckets/"+bucket.ID+"/credentials", map[string]string{"accessKeyId": "rotated-access-key", "secretAccessKey": "rotated-secret-key"})
	if w.Code != http.StatusOK || bytes.Contains(w.Body.Bytes(), []byte("rotated-access-key")) || bytes.Contains(w.Body.Bytes(), []byte("rotated-secret-key")) {
		t.Fatalf("safe bucket rotation response is wrong: %d %s", w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &bucket); err != nil || bucket.CredentialVersion != 2 {
		t.Fatalf("bucket credential version did not advance: %#v %v", bucket, err)
	}
	var encryptedSecret []byte
	if err = store.DB.QueryRow(ctx, `SELECT ciphertext FROM service_variables WHERE service_id=$1 AND name='UPLOADS_SECRET_ACCESS_KEY'`, application.ID).Scan(&encryptedSecret); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encryptedSecret, []byte("rotated-secret-key")) {
		t.Fatal("rotated bucket secret was stored as plaintext")
	}
	plainSecret, decryptErr := store.Cipher.Open(encryptedSecret, "variable:"+application.ID+":UPLOADS_SECRET_ACCESS_KEY")
	if decryptErr != nil || plainSecret != "rotated-secret-key" {
		t.Fatalf("connected bucket variable was not rotated: %q %v", plainSecret, decryptErr)
	}
	w = request(http.MethodDelete, "/api/buckets/"+bucket.ID+"/bindings/"+application.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("bucket unbind response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodGet, "/api/services/"+application.ID+"/variables", nil)
	if bytes.Contains(w.Body.Bytes(), []byte("UPLOADS_")) {
		t.Fatalf("bucket variables remained after disconnect: %s", w.Body.String())
	}
	w = request(http.MethodPut, "/api/buckets/"+bucket.ID+"/bindings/"+application.ID, map[string]string{"variablePrefix": "UPLOADS"})
	if w.Code != http.StatusOK {
		t.Fatalf("bucket rebind response: %d %s", w.Code, w.Body.String())
	}

	w = request(http.MethodGet, canvasPath, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("canvas response: %d %s", w.Code, w.Body.String())
	}
	var graph deployment.CanvasGraph
	if err = json.Unmarshal(w.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Resources) != 10 {
		t.Fatalf("expected app, worker, cron, two databases, four volumes and a bucket; got %#v", graph.Resources)
	}
	if len(graph.Links) != 7 {
		t.Fatalf("expected four attachments, two database references and a bucket binding; got %#v", graph.Links)
	}
	var appResource, databaseResource, redisResource *deployment.CanvasResource
	for index := range graph.Resources {
		resource := &graph.Resources[index]
		switch resource.ID {
		case application.ID:
			appResource = resource
		case database.ID:
			databaseResource = resource
		case redis.ID:
			redisResource = resource
		}
	}
	if appResource == nil || appResource.Kind != "service" || appResource.WorkloadMode != "web" || appResource.SourceType != "empty" {
		t.Fatalf("application projection is wrong: %#v", appResource)
	}
	if databaseResource == nil || databaseResource.Kind != "database" || databaseResource.Template != "postgres" || databaseResource.SourceType != "template" {
		t.Fatalf("database projection is wrong: %#v", databaseResource)
	}
	if redisResource == nil || redisResource.Kind != "database" || redisResource.Template != "redis" || redisResource.TemplateVersion != "8.2.2" || redisResource.PrivateAddress != "db-"+redis.ID+":6379" {
		t.Fatalf("redis projection is wrong: %#v", redisResource)
	}
	var bucketResource *deployment.CanvasResource
	for index := range graph.Resources {
		if graph.Resources[index].ID == bucket.ID {
			bucketResource = &graph.Resources[index]
		}
	}
	if bucketResource == nil || bucketResource.Status != "connected" || bucketResource.Endpoint != "http://minio:9000" || bucketResource.BucketName != "app-uploads" || bucketResource.CredentialVersion != 2 {
		t.Fatalf("bucket projection is wrong: %#v", bucketResource)
	}
	if !slices.ContainsFunc(graph.Links, func(link deployment.CanvasLink) bool {
		return link.Kind == "bucket-binding" && link.From == "service:"+application.ID && link.To == "bucket:"+bucket.ID && link.Label == "UPLOADS_*"
	}) {
		t.Fatalf("bucket binding link is missing: %#v", graph.Links)
	}

	position := deployment.CanvasPositionUpdate{ResourceKey: "service:" + application.ID, X: 444, Y: 222}
	w = request(http.MethodPut, canvasPath+"/layout", map[string]any{"positions": []deployment.CanvasPositionUpdate{position}})
	if w.Code != http.StatusOK {
		t.Fatalf("layout response: %d %s", w.Code, w.Body.String())
	}
	w = request(http.MethodGet, canvasPath, nil)
	if err = json.Unmarshal(w.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	for _, resource := range graph.Resources {
		if resource.Key == position.ResourceKey && resource.Position != (deployment.CanvasPosition{X: 444, Y: 222}) {
			t.Fatalf("layout was not persisted: %#v", resource.Position)
		}
	}
	w = request(http.MethodPut, canvasPath+"/layout", map[string]any{"positions": []deployment.CanvasPositionUpdate{{ResourceKey: "service:missing", X: 1, Y: 1}}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown resource layout accepted: %d", w.Code)
	}
}
