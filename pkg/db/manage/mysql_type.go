package manage

type MysqlConfig struct {
	GrantSuperPrivilege bool   `json:"grant_super_privilege"`
	CharacterSet        string `json:"character_set"`
}
