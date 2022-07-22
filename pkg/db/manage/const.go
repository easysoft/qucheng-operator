package manage

const (
	backupRoot = "/tmp/backup"

	resourceType     = "mysql"
	backupNameFormat = resourceType + "." + "%s.%s"
)
