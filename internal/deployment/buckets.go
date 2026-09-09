package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

type bucketCredentials struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

var bucketNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,61}[A-Za-z0-9]$`)
var bucketPrefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)

func (spec *BucketSpec) Validate() error {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Environment = strings.TrimSpace(spec.Environment)
	spec.Endpoint = strings.TrimRight(strings.TrimSpace(spec.Endpoint), "/")
	spec.Region = strings.TrimSpace(spec.Region)
	spec.BucketName = strings.TrimSpace(spec.BucketName)
	spec.AccessKeyID = strings.TrimSpace(spec.AccessKeyID)
	if spec.Environment == "" {
		spec.Environment = "production"
	}
	if !ValidName(spec.Name) || !bucketNamePattern.MatchString(spec.BucketName) {
		return errors.New("use a valid display name and a 3–63 character S3 bucket name")
	}
	u, err := url.Parse(spec.Endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("endpoint must be an HTTP(S) origin without credentials, query or path")
	}
	if len(spec.Endpoint) > 2048 || len(spec.Region) > 100 {
		return errors.New("check the endpoint and region lengths")
	}
	if err = ValidateBucketCredentials(spec.AccessKeyID, spec.SecretAccessKey); err != nil {
		return err
	}
	return nil
}

func ValidateBucketCredentials(accessKey, secretKey string) error {
	accessKey = strings.TrimSpace(accessKey)
	if len(accessKey) < 3 || len(accessKey) > 256 || len(secretKey) < 8 || len(secretKey) > 1024 || strings.ContainsRune(accessKey+secretKey, 0) {
		return errors.New("check the S3 credential lengths")
	}
	return nil
}

func (s *Store) CreateBucket(ctx context.Context, project string, spec BucketSpec) (Bucket, error) {
	var bucket Bucket
	if err := spec.Validate(); err != nil {
		return bucket, err
	}
	if s.Cipher == nil {
		return bucket, errors.New("encryption key unavailable")
	}
	bucket = Bucket{ID: ID(), ProjectID: project, Environment: spec.Environment, Name: spec.Name, Provider: "s3", Endpoint: spec.Endpoint, Region: spec.Region, BucketName: spec.BucketName, ForcePathStyle: spec.ForcePathStyle, CredentialVersion: 1}
	plain, _ := json.Marshal(bucketCredentials{AccessKeyID: spec.AccessKeyID, SecretAccessKey: spec.SecretAccessKey})
	sealed, err := s.Cipher.Seal(string(plain), "bucket:"+bucket.ID+":credentials")
	if err != nil {
		return Bucket{}, err
	}
	err = s.DB.QueryRow(ctx, `INSERT INTO buckets(id,project_id,environment,name,provider,endpoint,region,credentials,bucket_name,force_path_style)
	 VALUES($1,$2,$3,$4,'s3',$5,$6,$7,$8,$9) RETURNING created_at,updated_at`, bucket.ID, bucket.ProjectID, bucket.Environment, bucket.Name, bucket.Endpoint, bucket.Region, sealed, bucket.BucketName, bucket.ForcePathStyle).Scan(&bucket.CreatedAt, &bucket.UpdatedAt)
	return bucket, err
}

func bucketVariables(prefix string, bucket Bucket, credentials bucketCredentials) map[string]string {
	return map[string]string{
		prefix + "_BUCKET":            bucket.BucketName,
		prefix + "_ENDPOINT":          bucket.Endpoint,
		prefix + "_REGION":            bucket.Region,
		prefix + "_ACCESS_KEY_ID":     credentials.AccessKeyID,
		prefix + "_SECRET_ACCESS_KEY": credentials.SecretAccessKey,
		prefix + "_FORCE_PATH_STYLE":  map[bool]string{true: "true", false: "false"}[bucket.ForcePathStyle],
	}
}

func readBucket(row pgx.Row) (Bucket, []byte, error) {
	var bucket Bucket
	var sealed []byte
	err := row.Scan(&bucket.ID, &bucket.ProjectID, &bucket.Environment, &bucket.Name, &bucket.Provider, &bucket.Endpoint, &bucket.Region, &bucket.BucketName, &bucket.ForcePathStyle, &bucket.CredentialVersion, &sealed, &bucket.CreatedAt, &bucket.UpdatedAt)
	return bucket, sealed, err
}

const bucketColumns = `id,project_id,environment,name,provider,endpoint,region,bucket_name,force_path_style,credential_version,credentials,created_at,updated_at`

func (s *Store) openBucketCredentials(bucket Bucket, sealed []byte) (bucketCredentials, error) {
	var credentials bucketCredentials
	if s.Cipher == nil {
		return credentials, errors.New("encryption key unavailable")
	}
	plain, err := s.Cipher.Open(sealed, "bucket:"+bucket.ID+":credentials")
	if err == nil {
		err = json.Unmarshal([]byte(plain), &credentials)
	}
	return credentials, err
}

func (s *Store) bindBucketVariables(ctx context.Context, tx pgx.Tx, service, prefix string, bucket Bucket, credentials bucketCredentials) error {
	variables := bucketVariables(prefix, bucket, credentials)
	for name, value := range variables {
		sealed, err := s.Cipher.Seal(value, "variable:"+service+":"+name)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO service_variables(service_id,name,ciphertext) VALUES($1,$2,$3)
		 ON CONFLICT(service_id,name) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,updated_at=now()`, service, name, sealed); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BindBucket(ctx context.Context, id, service, prefix string) error {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if !bucketPrefixPattern.MatchString(prefix) {
		return errors.New("variable prefix must start with a letter and use up to 32 uppercase letters, numbers or underscores")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	bucket, sealed, err := readBucket(tx.QueryRow(ctx, `SELECT `+bucketColumns+` FROM buckets WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM services WHERE id=$1 AND project_id=$2 AND environment=$3 AND settings->>'kind'='http')`, service, bucket.ProjectID, bucket.Environment).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("choose an application in the same project and environment")
	}
	var currentPrefix string
	currentErr := tx.QueryRow(ctx, `SELECT variable_prefix FROM bucket_bindings WHERE bucket_id=$1 AND service_id=$2`, id, service).Scan(&currentPrefix)
	if currentErr == nil && currentPrefix != prefix {
		return errors.New("disconnect this bucket before changing its variable prefix")
	}
	if currentErr != nil && !errors.Is(currentErr, pgx.ErrNoRows) {
		return currentErr
	}
	var existingBucket string
	prefixErr := tx.QueryRow(ctx, `SELECT bucket_id FROM bucket_bindings WHERE service_id=$1 AND variable_prefix=$2`, service, prefix).Scan(&existingBucket)
	if prefixErr == nil && existingBucket != id {
		return errors.New("that application already uses this variable prefix for another bucket")
	}
	if prefixErr != nil && !errors.Is(prefixErr, pgx.ErrNoRows) {
		return prefixErr
	}
	names := make([]string, 0, 6)
	for name := range bucketVariables(prefix, bucket, bucketCredentials{}) {
		names = append(names, name)
	}
	if errors.Is(currentErr, pgx.ErrNoRows) {
		var collisions int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM service_variables WHERE service_id=$1 AND name=ANY($2)`, service, names).Scan(&collisions); err != nil {
			return err
		}
		if collisions != 0 {
			return errors.New("one or more generated variable names already exist; choose another prefix")
		}
		var variableCount int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM service_variables WHERE service_id=$1`, service).Scan(&variableCount); err != nil {
			return err
		}
		if variableCount+len(names) > 64 {
			return errors.New("bucket variables would exceed the 64-variable service limit")
		}
	}
	credentials, err := s.openBucketCredentials(bucket, sealed)
	if err != nil {
		return err
	}
	if err = s.bindBucketVariables(ctx, tx, service, prefix, bucket, credentials); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO bucket_bindings(bucket_id,service_id,variable_prefix) VALUES($1,$2,$3)
	 ON CONFLICT(bucket_id,service_id) DO UPDATE SET variable_prefix=EXCLUDED.variable_prefix,updated_at=now()`, id, service, prefix); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RotateBucketCredentials(ctx context.Context, id, accessKey, secretKey string) (Bucket, error) {
	accessKey = strings.TrimSpace(accessKey)
	if err := ValidateBucketCredentials(accessKey, secretKey); err != nil {
		return Bucket{}, err
	}
	if s.Cipher == nil {
		return Bucket{}, errors.New("encryption key unavailable")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Bucket{}, err
	}
	defer tx.Rollback(ctx)
	bucket, _, err := readBucket(tx.QueryRow(ctx, `SELECT `+bucketColumns+` FROM buckets WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return Bucket{}, err
	}
	credentials := bucketCredentials{AccessKeyID: accessKey, SecretAccessKey: secretKey}
	plain, _ := json.Marshal(credentials)
	sealed, err := s.Cipher.Seal(string(plain), "bucket:"+id+":credentials")
	if err != nil {
		return Bucket{}, err
	}
	rows, err := tx.Query(ctx, `SELECT service_id,variable_prefix FROM bucket_bindings WHERE bucket_id=$1 ORDER BY service_id`, id)
	if err != nil {
		return Bucket{}, err
	}
	type binding struct{ service, prefix string }
	bindings := []binding{}
	for rows.Next() {
		var item binding
		if err = rows.Scan(&item.service, &item.prefix); err != nil {
			rows.Close()
			return Bucket{}, err
		}
		bindings = append(bindings, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Bucket{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE buckets SET credentials=$2,credential_version=credential_version+1,updated_at=now() WHERE id=$1`, id, sealed); err != nil {
		return Bucket{}, err
	}
	bucket.CredentialVersion++
	for _, item := range bindings {
		if err = s.bindBucketVariables(ctx, tx, item.service, item.prefix, bucket, credentials); err != nil {
			return Bucket{}, err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT created_at,updated_at FROM buckets WHERE id=$1`, id).Scan(&bucket.CreatedAt, &bucket.UpdatedAt); err != nil {
		return Bucket{}, err
	}
	return bucket, tx.Commit(ctx)
}

func (s *Store) UnbindBucket(ctx context.Context, id, service string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var prefix string
	if err = tx.QueryRow(ctx, `DELETE FROM bucket_bindings WHERE bucket_id=$1 AND service_id=$2 RETURNING variable_prefix`, id, service).Scan(&prefix); err != nil {
		return err
	}
	names := make([]string, 0, 6)
	for name := range bucketVariables(prefix, Bucket{}, bucketCredentials{}) {
		names = append(names, name)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM service_variables WHERE service_id=$1 AND name=ANY($2)`, service, names); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
