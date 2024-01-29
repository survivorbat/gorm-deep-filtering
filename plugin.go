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

	createDeepFilterRecursively(exp.Exprs, db)

	return
}

func createDeepFilterRecursively(exprs []clause.Expression, db *gorm.DB) {
	for index, cond := range exprs {
		switch cond := cond.(type) {
		case clause.AndConditions:
			createDeepFilterRecursively(exprs[index].(clause.AndConditions).Exprs, db)

		case clause.Eq:
			switch value := cond.Value.(type) {
			case map[string]any:
				concreteType := ensureNotASlice(reflect.TypeOf(db.Statement.Model))
				inputObject := ensureConcrete(reflect.New(concreteType)).Interface()

				applied, err := AddDeepFilters(db.Session(&gorm.Session{NewDB: true}), inputObject, map[string]any{cond.Column.(string): value})

				if err != nil {
					_ = db.AddError(err)
					return
				}

				// Replace the map filter with the newly created deep-filter
				exprs[index] = applied.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs[0]
			}
		}
	}
}
