package deepgorm

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/survivorbat/go-tsyncmap"
	gormqonvert "github.com/survivorbat/gorm-query-convert"
	"gorm.io/gorm/schema"

	"gorm.io/gorm"
)

var (
	// Cache mechanism for reflecting database structs, reflection is slow, so we
	// cache results for quick lookups. Just remember to reset it in unit tests ;-)

	// cacheDatabaseMap map[string]map[string]*nestedType{}
	cacheDatabaseMap = tsyncmap.Map[string, map[string]*nestedType]{}

	// schemaCache is for gorm's schema.Parse
	schemaCache = sync.Map{}

	// ErrFieldDoesNotExist is returned if the Where condition contains unknown fields
	ErrFieldDoesNotExist = errors.New("field does not exist")
)

// AddDeepFilters / addDeepFilter godoc
//
// Gorm supports the following filtering:
//
//	type Person struct {
//		Name string
//	}
//
//	map[string]any{
//		"name": "Jake"
//	}
//
// Which will return a list of people that are named 'Jake'. This is great for simple filtering
// but for more nested versions like the following it becomes problematic.
//
//	type Group struct {
//		IDs int
//		Name string
//	}
//
//	type Person struct {
//		Name string
//		Group Group
//		GroupRef int
//	}
//
// // Get all the users belonging to 'some group'
//
//	map[string]any{
//		"group": map[string]any{
//			"name": "some group",
//		},
//	}
//
// Gorm does not understand that we expected to filter users based on their group, it's
// not capable of doing that automatically. For this we need to use subqueries. Find more info here:
// https://gorm.io/docs/advanced_query.html
//
// This function is constructed to automatically convert those nested maps ("group": map[string]...) into
// subqueries. In order to do this, it takes the following steps:
//
//  1. Get all the struct-type fields from the incoming 'object', ignore all simple types and interfaces
//  2. Loop through all the key/values in the incoming map
//  3. Add all the simple types to a simpleMap, GORM can handle these,
//     For all the special (nested) structs, add a subquery that uses WHERE on the subquery.
//  4. Add the simple filters to the query and return it.
func AddDeepFilters(db *gorm.DB, objectType any, filters ...map[string]any) (*gorm.DB, error) {
	schemaInfo, err := schema.Parse(objectType, &schemaCache, db.NamingStrategy)
	if err != nil {
		return nil, err
	}

	relationalTypesInfo := getDatabaseFieldsOfType(db.NamingStrategy, schemaInfo)

	simpleFilter := map[string]any{}
	totalFilterString := ""
	functionRegex := regexp.MustCompile(`.*\((.*?)\)+`)
	qonvertMap := map[string]string{}

	if _, ok := db.Plugins[gormqonvert.New(gormqonvert.CharacterConfig{}).Name()]; ok {
		qonvertPlugin := db.Plugins[gormqonvert.New(gormqonvert.CharacterConfig{}).Name()]
		qonvertPluginConfig := reflect.ValueOf(qonvertPlugin).Elem().FieldByName("config")
		qonvertMap[qonvertPluginConfig.FieldByName("GreaterThanPrefix").String()] = ">"
		qonvertMap[qonvertPluginConfig.FieldByName("GreaterOrEqualToPrefix").String()] = ">="
		qonvertMap[qonvertPluginConfig.FieldByName("LessThanPrefix").String()] = "<"
		qonvertMap[qonvertPluginConfig.FieldByName("LessOrEqualToPrefix").String()] = "<="
		qonvertMap[qonvertPluginConfig.FieldByName("NotEqualToPrefix").String()] = "!="
		qonvertMap[qonvertPluginConfig.FieldByName("LikePrefix").String()] = "%s LIKE '%%%s%%'"
		qonvertMap[qonvertPluginConfig.FieldByName("NotLikePrefix").String()] = "%s NOT LIKE '%%%s%%'"
	}

	// Go through the filters
	for _, filterObject := range filters {
		// Go through all the keys of the filters
		for fieldName, givenFilter := range filterObject {
			filterString := ""
			switch givenFilter.(type) {
			// WithFilters for relational objects
			case map[string]any:
				fieldInfo, ok := relationalTypesInfo[fieldName]

				if !ok {
					return nil, fmt.Errorf("failed to add filters for '%s.%s': %w", schemaInfo.Table, fieldName, ErrFieldDoesNotExist)
				}

				// We have 2 db objects because if we use 'result' to create subqueries it will cause a stackoverflow.
				query, err := addDeepFilter(db, fieldInfo, givenFilter)
				if err != nil {
					return nil, err
				}

				db = query

			// Simple filters (string, int, bool etc.)
			default:
				// If the simple filter contains a function, build the query different
				if functionRegex.MatchString(fieldName) {
					checkFieldName := functionRegex.ReplaceAllString(fieldName, "$1")
					if _, ok := schemaInfo.FieldsByDBName[checkFieldName]; !ok {
						return nil, fmt.Errorf("failed to add filters for '%s.%s': %w", schemaInfo.Table, checkFieldName, ErrFieldDoesNotExist)
					}

					if _, ok := givenFilter.([]string); ok {
						for _, filter := range givenFilter.([]string) {
							containedQonvert := false
							qonvertOperator := ""
							qonvertFilter := ""
							for qonvertField, qonvertValue := range qonvertMap {
								// Find longest possible prefix match in filter
								// (e.g. to make sure <= is fully matched and not overwritten by <)
								if strings.HasPrefix(filter, qonvertField) && len(qonvertField) > len(qonvertFilter) {
									qonvertOperator = qonvertValue
									qonvertFilter = qonvertField
									containedQonvert = true
								}
							}

							if !containedQonvert {
								if filterString == "" {
									filterString = fmt.Sprintf("%s = '%s'", fieldName, filter)
								} else {
									filterString = fmt.Sprintf("%s OR %s", filterString, fmt.Sprintf("%s = '%s'", fieldName, filter))
								}
							} else {
								filter = strings.Replace(filter, qonvertFilter, "", 1)
								if strings.Contains(qonvertOperator, "%") {
									if filterString == "" {
										filterString = fmt.Sprintf(qonvertOperator, fieldName, filter)
									} else {
										filterString = fmt.Sprintf("%s OR %s", filterString, fmt.Sprintf(qonvertOperator, fieldName, filter))
									}
								} else {
									if filterString == "" {
										filterString = prepareFilterValue(fieldName, qonvertOperator, filter)
									} else {
										filterString = fmt.Sprintf("%s OR %s", filterString, prepareFilterValue(fieldName, qonvertOperator, filter))
									}
								}
							}
						}

						filterString = fmt.Sprintf("(%s)", filterString)
						if totalFilterString != "" {
							totalFilterString += " AND "
						}
						totalFilterString += filterString
						break
					}

					if _, ok := givenFilter.([]int); ok {
						for _, filter := range givenFilter.([]int) {
							if filterString == "" {
								filterString = fmt.Sprintf("%s = %d", fieldName, filter)
							} else {
								filterString = fmt.Sprintf("%s OR %s", filterString, fmt.Sprintf("%s = %d", fieldName, filter))
							}
						}

						filterString = fmt.Sprintf("(%s)", filterString)
						if totalFilterString != "" {
							totalFilterString += " AND "
						}
						totalFilterString += filterString
						break
					}

					containedQonvert := false
					qonvertOperator := ""
					qonvertFilter := ""
					for qonvertField, qonvertValue := range qonvertMap {
						// Find longest possible prefix match in filter
						// (e.g. to make sure <= is fully matched and not overwritten by <)
						if filterStrCast, castOk := givenFilter.(string); castOk && strings.HasPrefix(filterStrCast, qonvertField) && len(qonvertField) > len(qonvertFilter) {
							qonvertOperator = qonvertValue
							qonvertFilter = qonvertField
							containedQonvert = true
						}

					}

					if containedQonvert {
						givenFilter = strings.Replace(givenFilter.(string), qonvertFilter, "", 1)
						if strings.Contains(qonvertOperator, "%") {
							if filterString == "" {
								filterString = fmt.Sprintf(qonvertOperator, fieldName, givenFilter)
							} else {
								filterString = fmt.Sprintf("%s OR %s", filterString, fmt.Sprintf(qonvertOperator, fieldName, givenFilter))
							}
						} else {
							filterString = prepareFilterValue(fieldName, qonvertOperator, givenFilter.(string))
						}
					}

					if filterString == "" {
						filterString = prepareFilterValueCast(fieldName, "=", givenFilter)
					}

					filterString = fmt.Sprintf("(%s)", filterString)
					if totalFilterString != "" {
						totalFilterString += " AND "
					}
					totalFilterString += filterString
					break
				}

				if _, ok := schemaInfo.FieldsByDBName[fieldName]; !ok {
					return nil, fmt.Errorf("failed to add filters for '%s.%s': %w", schemaInfo.Table, fieldName, ErrFieldDoesNotExist)
				}
				simpleFilter[fieldName] = givenFilter
			}
		}
	}

	// Add simple filters
	if totalFilterString != "" {
		db = db.Where(totalFilterString)
	}
	db = db.Where(simpleFilter)

	return db, nil
}

// nestedType Wrapper object used to create subqueries.
//
// NOTICE: We can only do simple many-to-many's with 2 ids right now, I currently (15-06-2021) see no reason
// to add even more advanced options.
type nestedType struct {
	// An empty instance of the object, used in db.Model(...)
	fieldStructInstance any
	fieldForeignKey     string

	// Whether this is a manyToOne, oneToMany or manyToMany. oneToOne is taken care of automatically.
	relationType string

	/////////////////////////
	// Many to Many fields //
	/////////////////////////

	// The name of the join table
	manyToManyTable string

	// The destination field from destinationManyToManyStructInstance
	destinationManyToManyForeignKey string
}

// iKind is an abstraction of reflect.Value and reflect.Type that allows us to make ensureConcrete generic.
type iKind[T any] interface {
	Kind() reflect.Kind
	Elem() T
}

// prepareFilterValue checks if the given filter can be converted to an int or bool and gives back the correct SQL value for it
func prepareFilterValue(fieldName string, operator string, filterValue string) string {
	if value, err := strconv.Atoi(filterValue); err == nil {
		return fmt.Sprintf("%s %s %d", fieldName, operator, value)
	}

	if value, err := strconv.ParseBool(filterValue); err == nil {
		return fmt.Sprintf("%s %s %t", fieldName, operator, value)
	}

	return fmt.Sprintf("%s %s '%s'", fieldName, operator, filterValue)
}

// prepareFilterValue checks if the given filter can be converted to an int or bool and gives back the correct SQL value for it
func prepareFilterValueCast(fieldName string, operator string, filterValue any) string {
	if filterIntCast, castOk := filterValue.(int); castOk {
		return fmt.Sprintf("%s %s %d", fieldName, operator, filterIntCast)
	}
	if filterBoolCast, castOk := filterValue.(bool); castOk {
		return fmt.Sprintf("%s %s %t", fieldName, operator, filterBoolCast)
	}

	return fmt.Sprintf("%s %s '%s'", fieldName, operator, filterValue.(string))
}

// ensureConcrete ensures that the given value is a value and not a pointer, if it is, convert it to its element type
func ensureConcrete[T iKind[T]](value T) T {
	if value.Kind() == reflect.Ptr {
		return ensureConcrete(value.Elem())
	}

	return value
}

// ensureNotASlice Ensures that the given value is not a slice, if it is a slice, we use Elem()
// For example: Type []*string will return string. This one is not generic because it doesn't work
// well with reflect.Value.
func ensureNotASlice(value reflect.Type) reflect.Type {
	result := ensureConcrete(value)

	if result.Kind() == reflect.Slice {
		return ensureNotASlice(result.Elem())
	}

	return result
}

// getInstanceAndRelationOfField Since db.Model(...) requires an instance, we use this function to instantiate a field type
// and retrieve what kind of relation we assume the object has.
func getInstanceAndRelationOfField(fieldType reflect.Type) (any, string) {
	valueType := ensureConcrete(fieldType)

	switch valueType.Kind() {
	// If the given field is a struct, we can safely say it's a oneToMany, we instantiate it
	// using reflect.New and return it as an object.
	case reflect.Struct:
		return reflect.New(valueType).Interface(), "oneToMany"

	// If the given field is a slice, it can be either manyToOne or manyToMany. We figure out what
	// kind of slice it is and use reflect.New to return it as an object
	case reflect.Slice:
		elementType := ensureNotASlice(valueType)
		return reflect.New(elementType).Interface(), "manyToOne"

	default:
		return nil, ""
	}
}

// getNestedType Returns information about the struct field in a nestedType object. Used to figure out
// what database tables need to be queried.
func getNestedType(naming schema.Namer, dbField *schema.Field, ofType reflect.Type) (*nestedType, error) {
	// Get empty instance for db.Model() of the given field
	sourceStructType, relationType := getInstanceAndRelationOfField(dbField.FieldType)

	result := &nestedType{
		relationType:        relationType,
		fieldStructInstance: sourceStructType,
	}

	sourceForeignKey, ok := dbField.TagSettings["FOREIGNKEY"]
	if ok {
		result.fieldForeignKey = naming.ColumnName(dbField.Schema.Table, sourceForeignKey)
		return result, nil
	}

	// No foreign key found, then it must be a manyToMany
	manyToMany, ok := dbField.TagSettings["MANY2MANY"]

	if !ok {
		return nil, fmt.Errorf("no 'foreignKey:...' or 'many2many:...' found in field %s", dbField.Name)
	}

	// Woah it's a many-to-many!
	result.relationType = "manyToMany"
	result.manyToManyTable = manyToMany

	// Based on the type we can just put _id behind it, again this only works with simple many-to-many structs
	result.fieldForeignKey = naming.ColumnName(dbField.Schema.Table, ensureNotASlice(dbField.FieldType).Name()) + "_id"

	// Now the other table that we're getting information from.
	result.destinationManyToManyForeignKey = naming.ColumnName(dbField.Schema.Table, ofType.Name()) + "_id"

	return result, nil
}

// getDatabaseFieldsOfType godoc
// Helper method used in AddDeepFilters to get nestedType objects for specific fields.
// For example, the following struct.
//
//	type Tag struct {
//		IDs uuid.UUID
//	}
//
//	type SimpleStruct1 struct {
//		Name string
//		TagRef uuid.UUID
//		Tag Tag `gorm:"foreignKey:TagRef"`
//	}
//
// Now when we call getDatabaseFieldsOfType(SimpleStruct1{}) it will return the following
// map of items.
//
//	{
//		"nestedstruct": {
//			fieldStructInstance: Tag{},
//			fieldForeignKey: "NestedStructRef",
//			relationType: "oneToMany"
//		}
//	}
func getDatabaseFieldsOfType(naming schema.Namer, schemaInfo *schema.Schema) map[string]*nestedType {
	// First get all the information of the to-be-reflected object
	reflectType := ensureConcrete(schemaInfo.ModelType)
	reflectTypeName := reflectType.Name()

	// The len(dbFields) check is needed here because when running the unit tests
	// it fell into a race condition where it had the map key already stored but not the value yet.
	// Resulting in some fields reported falsely as non existent
	if dbFields, ok := cacheDatabaseMap.Load(reflectTypeName); ok && len(dbFields) != 0 {
		return dbFields
	}

	var resultNestedType = map[string]*nestedType{}

	for _, fieldInfo := range schemaInfo.FieldsByName {
		// Not interested in these
		if kind := ensureConcrete(fieldInfo.FieldType).Kind(); kind != reflect.Struct && kind != reflect.Slice {
			continue
		}

		nestedTypeResult, err := getNestedType(naming, fieldInfo, reflectType)
		if err != nil {
			continue
		}

		resultNestedType[naming.ColumnName(schemaInfo.Table, fieldInfo.Name)] = nestedTypeResult
	}

	// Add to cache
	cacheDatabaseMap.Store(reflectTypeName, resultNestedType)

	return resultNestedType
}

// AddDeepFilters / addDeepFilter godoc
// Refer to AddDeepFilters.
func addDeepFilter(db *gorm.DB, fieldInfo *nestedType, filter any) (*gorm.DB, error) {
	cleanDB := db.Session(&gorm.Session{NewDB: true})

	switch fieldInfo.relationType {
	case "oneToMany":
		// SELECT * FROM <table> WHERE fieldInfo.fieldForeignKey IN (SELECT id FROM fieldInfo.fieldStructInstance WHERE givenFilter)
		whereQuery := fmt.Sprintf("%s IN (?)", fieldInfo.fieldForeignKey)
		subQuery, err := AddDeepFilters(cleanDB, fieldInfo.fieldStructInstance, filter.(map[string]any))

		if err != nil {
			return nil, err
		}

		return db.Where(whereQuery, cleanDB.Model(fieldInfo.fieldStructInstance).Select("id").Where(subQuery)), nil

	case "manyToOne":
		// SELECT * FROM <table> WHERE id IN (SELECT fieldInfo.fieldStructInstance FROM fieldInfo.fieldStructInstance WHERE filter)
		subQuery, err := AddDeepFilters(cleanDB, fieldInfo.fieldStructInstance, filter.(map[string]any))

		if err != nil {
			return nil, err
		}

		return db.Where("id IN (?)", cleanDB.Model(fieldInfo.fieldStructInstance).Select(fieldInfo.fieldForeignKey).Where(subQuery)), nil

	case "manyToMany":
		// SELECT * FROM <table> WHERE id IN (SELECT <table>_id FROM fieldInfo.fieldForeignKey WHERE <other_table>_id IN (SELECT id FROM <other_table> WHERE givenFilter))

		// The one that connects the objects
		subWhere := fmt.Sprintf("%s IN (?)", fieldInfo.fieldForeignKey)
		subQuery, err := AddDeepFilters(cleanDB, fieldInfo.fieldStructInstance, filter.(map[string]any))

		if err != nil {
			return nil, err
		}

		return db.Where("id IN (?)", cleanDB.Table(fieldInfo.manyToManyTable).Select(fieldInfo.destinationManyToManyForeignKey).Where(subWhere, cleanDB.Model(fieldInfo.fieldStructInstance).Select("id").Where(subQuery))), nil
	}

	return nil, fmt.Errorf("relationType '%s' unknown", fieldInfo.relationType)
}
