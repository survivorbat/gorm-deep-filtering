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

// Wildcards enables wildcard use for LIKE queries in input filters, converting *'s to %'s for LIKE queries.
// NOTICE: This feature is experimental and might be changed in the future (different symbol for example).
func Wildcards() func(*deepGorm) {
	return func(cfg *deepGorm) {
		cfg.wildcards = true
	}
}

type deepGorm struct {
	wildcards bool
}

func (d *deepGorm) Name() string {
	return "deepgorm"
}

func (d *deepGorm) Initialize(db *gorm.DB) error {
	return db.Callback().Query().Before("gorm:query").Register("deepgorm:query", d.queryCallback)
}

func (d *deepGorm) queryCallback(db *gorm.DB) {
	exp, ok := db.Statement.Clauses["WHERE"].Expression.(clause.Where)
	if !ok {
		return
	}

	for index, cond := range exp.Exprs {
		switch cond := cond.(type) {
		case clause.Eq:
			switch value := cond.Value.(type) {
			case string:
				applyFilter(db, d.wildcards, index, cond, value)

			case map[string]any:
				applyFilter(db, d.wildcards, index, cond, value)
			}

		case clause.IN:
			applyFilter(db, d.wildcards, index, cond, cond.Values)
		}
	}

	return
}

func applyFilter[C clause.Eq | clause.IN](db *gorm.DB, wildcards bool, index int, cond C, value any) {
	concreteType := ensureNotASlice(reflect.TypeOf(db.Statement.Model))
	inputObject := ensureConcrete(reflect.New(concreteType)).Interface()

	var columnString string
	var ok bool

	switch column := any(cond).(type) {
	case clause.Eq:
		columnString, ok = column.Column.(string)
	case clause.IN:
		columnString, ok = column.Column.(string)
	}

	if !ok {
		return
	}

	applied, err := addDeepFilters(db.Session(&gorm.Session{NewDB: true}), inputObject, wildcards, map[string]any{columnString: value})

	if err != nil {
		_ = db.AddError(err)
		return
	}

	// Replace the map filter with the newly created deep-filter
	for _, expression := range applied.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs {
		db.Statement.Clauses["WHERE"].Expression.(clause.Where).Exprs[index] = expression
	}
}
