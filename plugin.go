package deepgorm

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
)

// Compile-time interface check
var _ gorm.Plugin = new(deepGorm)

// New creates a new instance of the plugin that can be registered in gorm.
func New() gorm.Plugin {
	return &deepGorm{}
}

type deepGorm struct {
}

func (d *deepGorm) Name() string {
	return "deepgorm"
}

func (d *deepGorm) Initialize(db *gorm.DB) error {
	return db.Callback().Query().Before("gorm:query").Register("deepgorm:query", queryCallback)
}

func queryCallback(db *gorm.DB) {
	exp, ok := db.Statement.Clauses["WHERE"].Expression.(clause.Where)
	if !ok {
		return
	}

	for index, cond := range exp.Exprs {
		switch cond := cond.(type) {
		case clause.Eq:
			switch value := cond.Value.(type) {
			case map[string]any:
				concreteType := ensureNotASlice(reflect.TypeOf(db.Statement.Model))

				inputObject := ensureConcrete(reflect.New(concreteType)).Interface()
				_, err := AddDeepFilters(db.Model(inputObject), inputObject, map[string]any{cond.Column.(string): value})

				if err != nil {
					_ = db.AddError(err)
					return
				}

				// Empty the WHERE clause so it doesn't get applied
				exp.Exprs = append(db.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs[:index], db.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs[index+1:]...)
			}
		}
	}
}
