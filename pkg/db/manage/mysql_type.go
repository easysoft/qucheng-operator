package manage

type MysqlConfig struct {
	GrantSuperPrivilege string `json:"grant_super_privilege,omitempty"`
	CharacterSet        string `json:"character_set,omitempty"`
}
