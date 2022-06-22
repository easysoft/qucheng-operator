package db

type InterFace interface {
	ParseAccessInfo() (*AccessInfo, error)
}

type AccessInfo struct {
	Host     string
	Port     int32
	User     string
	Password string
}
