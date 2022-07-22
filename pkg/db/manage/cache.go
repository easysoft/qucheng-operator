package manage

var _dbMetaPool = make(map[string]*DbMeta)

func getDbMetaFromCache(key string) *DbMeta {
	return _dbMetaPool[key]
}

func addDbMetaToCache(key string, meta *DbMeta) {
	_dbMetaPool[key] = meta
}
