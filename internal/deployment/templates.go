package deployment

import (
	"errors"
	"fmt"
)

const RedisImage = "redis@sha256:59b6e694653476de2c992937ebe1c64182af4728e54bb49e9b7a6c26614d8933"
const MySQLImage = "mysql@sha256:0426ec38c7a10aa45ba383887df7878f74ee70e2fd589c7b69207f3577901903"

type DataTemplate struct {
	Key         string `json:"key"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Port        int    `json:"port"`
	MountPath   string `json:"mountPath"`
	MemoryMB    int    `json:"memoryMB"`
}

type dataTemplateDefinition struct {
	DataTemplate
	Image        string
	Kind         string
	StartCommand string
	Variables    func(string) map[string]string
}

var dataTemplateCatalog = []dataTemplateDefinition{
	{DataTemplate: DataTemplate{Key: "postgres", Version: "17.6", Name: "PostgreSQL", Description: "Relational database with persistent storage", Port: 5432, MountPath: "/var/lib/postgresql/data", MemoryMB: 256}, Image: PostgresImage, Kind: "postgres", Variables: func(string) map[string]string {
		return map[string]string{"POSTGRES_USER": "app", "POSTGRES_DB": "app", "POSTGRES_PASSWORD": ID() + ID()}
	}},
	{DataTemplate: DataTemplate{Key: "redis", Version: "8.2.2", Name: "Redis", Description: "Cache, queue and key-value data with append-only persistence", Port: 6379, MountPath: "/data", MemoryMB: 128}, Image: RedisImage, Kind: "redis", StartCommand: `exec redis-server --appendonly yes --requirepass "$REDIS_PASSWORD"`, Variables: func(serviceID string) map[string]string {
		password := ID() + ID()
		host := "db-" + serviceID
		return map[string]string{"REDISHOST": host, "REDISUSER": "default", "REDISPORT": "6379", "REDIS_PASSWORD": password, "REDIS_URL": fmt.Sprintf("redis://default:%s@%s:6379/0", password, host)}
	}},
	{DataTemplate: DataTemplate{Key: "mysql", Version: "8.4.7", Name: "MySQL", Description: "Relational database with persistent InnoDB storage", Port: 3306, MountPath: "/var/lib/mysql", MemoryMB: 512}, Image: MySQLImage, Kind: "mysql", Variables: func(serviceID string) map[string]string {
		password := ID() + ID()
		rootPassword := ID() + ID()
		host := "db-" + serviceID
		return map[string]string{"MYSQL_ROOT_PASSWORD": rootPassword, "MYSQL_DATABASE": "app", "MYSQL_USER": "app", "MYSQL_PASSWORD": password, "MYSQL_URL": fmt.Sprintf("mysql://app:%s@%s:3306/app", password, host)}
	}},
}

func DataTemplates() []DataTemplate {
	items := make([]DataTemplate, len(dataTemplateCatalog))
	for i, template := range dataTemplateCatalog {
		items[i] = template.DataTemplate
	}
	return items
}

func ValidDataTemplate(key string) bool {
	_, err := dataTemplate(key, "")
	return err == nil
}

func dataTemplate(key, version string) (dataTemplateDefinition, error) {
	for _, template := range dataTemplateCatalog {
		if template.Key == key && (version == "" || template.Version == version) {
			return template, nil
		}
	}
	return dataTemplateDefinition{}, errors.New("unknown or unsupported data template version")
}

func IsDataKind(kind string) bool { return kind == "postgres" || kind == "redis" || kind == "mysql" }
