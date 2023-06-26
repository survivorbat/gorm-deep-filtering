package deepgorm

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
)

// Compile-time interface check
var _ gorm.Plugin = new(deepGorm)

// New creates a new instance of the plugin that can be registered in gorm. Options allow you to
// enable specific features.
func New(opts ...Option) *deepGorm {
	plugin := new(deepGorm)

	for _, opt := range opts {
		opt(plugin)
	}

	return plugin
}

// Option is used to enable features in the New function
type Option func(*deepGorm)

// DeepLike enables the ability to add `deepgorm:"like"` to the fields in your struct that you want to automatically
// use LIKE %value% for instead of the normal WHERE check.
func DeepLike() func(*deepGorm) {
	return func(cfg *deepGorm) {
		cfg.deepLike = true
	}
}

type deepGorm struct {
	deepLike bool
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

				applied, err := AddDeepFilters(db.Session(&gorm.Session{NewDB: true}), inputObject, map[string]any{cond.Column.(string): value})

				if err != nil {
					_ = db.AddError(err)
					return
				}

				// Replace the map filter with the newly created deep-filter
				db.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs[index] = applied.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs[0]
			}
		}
	}

	return
}
