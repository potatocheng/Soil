package orm

import (
	"Soil/orm/internal/model"
	"Soil/orm/internal/valuer"
)

// core 存放CRUD都需要东西，以提供给transaction和db使用, 在db的openDB中初始化
type core struct {
	r           model.Registry
	model       *model.Model
	valCreator  valuer.Creator
	dialect     Dialect
	middlewares []Middleware
}
