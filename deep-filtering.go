package deepgorm

import (
	"fmt"
	"github.com/stoewer/go-strcase"
	"gorm.io/gorm/schema"
	"reflect"
	"strings"
	"sync"

	"gorm.io/gorm"
)

var (
	// Cache mechanism for reflecting database structs, reflection is slow, so we
	// cache results for quick lookups. Just remember to reset it in unit tests ;-)

	// cacheDatabaseMap map[string]map[string]*nestedType{}
	cacheDatabaseMap = sync.Map{}
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
	// Special assignment so we don't override the input parameter since that could cause confusion.
	// We also need the 'clean' db object for subqueries.
	// Assign with a new session - to prevent overriding any prior config like `.Model(&Struct{})` later on when building queries with `.Table("Struct")`
	result := db

	relationalTypesInfo := getDatabaseFieldsOfType(objectType)

	simpleFilter := map[string]any{}
	table, _ := schema.Parse(objectType, &sync.Map{}, schema.NamingStrategy{})

	// Go through the filters
	for _, filterObject := range filters {
		// Go through all the keys of the filters
		for fieldName, givenFilter := range filterObject {
			switch givenFilter.(type) {
			// WithFilters for relational objects
			case map[string]any:
				fieldInfo, ok := relationalTypesInfo[fieldName]

				if !ok {
					return nil, fmt.Errorf("field '%v' does not exist", fieldName)
				}

				// We have 2 db objects because if we use 'result' to create subqueries it will cause a stackoverflow.
				query, err := addDeepFilter(db, result, fieldInfo, givenFilter)
				if err != nil {
					return nil, err
				}

				result = query

			// Simple filters (string, int, bool etc.)
			default:
				simpleFilter[table.Table+"."+fieldName] = givenFilter
			}
		}
	}

	// Add simple filters
	result = result.Where(simpleFilter)

	return result, nil
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

	// Since we're dealing with 2 struct types at the same time in a many-to-many situation, used in db.Model(...)
	destinationManyToManyStructInstance any
}

// getSpecificStructTagValue Gets the `tag:"key:xxx"` value from a struct field. For example.
//
//	type Nested struct {
//	  IDs int
//	}
//
//	type MyStruct struct {
//	   NestedStruct Nested `gorm:"foreignKey:NestedStructID"`
//	   NestedStructID
//	}
//
// Using getSpecifiedStructTagValue(field, "gorm", "foreignKey" will give you 'NestedStructID' which is what
// comes after the colon on 'NestedStruct Nested' in struct BObject.
func getSpecificStructTagValue(field reflect.StructField, tag string, key string) (string, error) {
	// Get the tag from the struct field, because we need to know what the foreign key is.
	tagContent, ok := field.Tag.Lookup(tag)

	if !ok {
		// If the tag doesn't exist, we don't know how to use a foreign key, so we just skip this one.
		// Originally this was an Errorf, but it logged a lot of errors for fields that didn't have a foreign key.
		return "", fmt.Errorf("tag '%v' not found in field '%v'", tag, field.Name)
	}

	tags := strings.Split(tagContent, ";")
	prefix := key + ":"

	for _, tagValue := range tags {
		if !strings.Contains(tagValue, prefix) {
			continue
		}

		return strings.TrimPrefix(tagValue, prefix), nil
	}

	return "", fmt.Errorf("key '%v' in tag '%v' not found", key, tag)
}

// iKind is an abstraction of reflect.Value and reflect.Type that allows us to make ensureConcrete generic.
type iKind[T any] interface {
	Kind() reflect.Kind
	Elem() T
}

// ensureConcrete ensures that the given value is a value and not a pointer, if it is, convert it to its element type
func ensureConcrete[T iKind[T]](value T) T {
	if value.Kind() == reflect.Ptr {
		return ensureConcrete(value.Elem())
	}

	return value
}

// ensureNotASlice upserts that the given value is not a slice, if it is a slice, we use Elem()
// For example: Type []*string will return string
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
		elementType := ensureConcrete(valueType.Elem())
		return reflect.New(elementType).Interface(), "manyToOne"
	}

	return nil, ""
}

// getNestedType godoc
// Return information about the struct field in a nestedType object. Used to figure out
// what database tables need to be queried.
func getNestedType(field reflect.StructField, ofType reflect.Type) (*nestedType, error) {
	// Get empty instance for db.Model() of the given field
	sourceStructType, relationType := getInstanceAndRelationOfField(field.Type)

	result := &nestedType{
		relationType:        relationType,
		fieldStructInstance: sourceStructType,
	}

	// Get the foreignKey
	sourceForeignKey, err := getSpecificStructTagValue(field, "gorm", "foreignKey")
	result.fieldForeignKey = strcase.SnakeCase(sourceForeignKey)

	// We found a foreign key, we're done :-)
	if err == nil {
		return result, nil
	}

	// No foreign key found, then it must be a manyToMany
	manyToMany, manyErr := getSpecificStructTagValue(field, "gorm", "many2many")

	if manyErr != nil {
		// No 'foreignKey' or 'many2many'
		return nil, fmt.Errorf("no 'foreignKey:...' or 'many2many:...' found in field %v", field.Name)
	}

	// Woah it's a many-to-many!
	result.relationType = "manyToMany"
	result.manyToManyTable = manyToMany

	// Based on the type we can just put _id behind it, again this only works with simple many-to-many structs
	result.fieldForeignKey = strcase.SnakeCase(ensureNotASlice(field.Type).Name()) + "_id"

	// Empty instance for db.Model()
	destinationStructInstance, _ := getInstanceAndRelationOfField(field.Type)

	// Now the other table that we're getting information from.
	result.destinationManyToManyForeignKey = strcase.SnakeCase(ofType.Name()) + "_id"
	result.destinationManyToManyStructInstance = destinationStructInstance

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
//	type SimpleStruct struct {
//		Name string
//		TagRef uuid.UUID
//		Tag Tag `gorm:"foreignKey:TagRef"`
//	}
//
// Now when we call getDatabaseFieldsOfType(SimpleStruct{}) it will return the following
// map of items.
//
//	{
//		"nestedstruct": {
//			fieldStructInstance: Tag{},
//			fieldForeignKey: "NestedStructRef",
//			relationType: "oneToMany"
//		}
//	}
func getDatabaseFieldsOfType(objectType any) map[string]*nestedType {
	// First get all the information of the to-be-reflected object
	reflectType := ensureConcrete(reflect.TypeOf(objectType))
	reflectTypeName := fmt.Sprintf("%T", objectType)

	if dbFields, ok := cacheDatabaseMap.Load(reflectType.Name()); ok {
		return dbFields.(map[string]*nestedType)
	}

	var resultNestedType = map[string]*nestedType{}

	// Iterate through fields
	for i := 0; i < reflectType.NumField(); i++ {
		// Get the field
		field := reflectType.Field(i)
		fieldType := field.Type
		fieldName := strcase.SnakeCase(field.Name)

		// Upsert if the type is a value.
		fieldType = ensureConcrete(fieldType)

		// If the type is NOT a struct, we're not interested and skip this one.
		if kind := fieldType.Kind(); kind != reflect.Struct && kind != reflect.Slice {
			continue
		}

		// Get nestedType object
		fieldInfo, err := getNestedType(field, reflectType)
		if err != nil {
			continue
		}

		resultNestedType[fieldName] = fieldInfo
	}

	// Add to cache
	cacheDatabaseMap.Store(reflectTypeName, resultNestedType)

	return resultNestedType
}

// AddDeepFilters / addDeepFilter godoc
// Refer to AddDeepFilters.
func addDeepFilter(cleanDB *gorm.DB, resultDB *gorm.DB, fieldInfo *nestedType, filter any) (*gorm.DB, error) {
	switch fieldInfo.relationType {
	case "oneToMany":
		whereQuery := fmt.Sprintf("%s IN (?)", fieldInfo.fieldForeignKey)

		// SELECT * FROM <table> WHERE fieldInfo.fieldForeignKey IN (SELECT id FROM fieldInfo.fieldStructInstance WHERE givenFilter)
		return resultDB.Where(whereQuery, cleanDB.Model(fieldInfo.fieldStructInstance).Select("id").Where(filter)), nil

	case "manyToOne":
		// SELECT * FROM <table> WHERE id IN (SELECT fieldInfo.fieldStructInstance FROM fieldInfo.fieldStructInstance WHERE filter)
		return resultDB.Where("id IN (?)", cleanDB.Model(fieldInfo.fieldStructInstance).Select(fieldInfo.fieldForeignKey).Where(filter)), nil

	case "manyToMany":
		// The one on the 'other' object
		subSubQuery := cleanDB.Model(fieldInfo.destinationManyToManyStructInstance).Select("id").Where(filter)

		// The one that connects the objects
		subWhere := fmt.Sprintf("%v IN (?)", fieldInfo.fieldForeignKey)
		subQuery := cleanDB.Table(fieldInfo.manyToManyTable).Select(fieldInfo.destinationManyToManyForeignKey).Where(subWhere, subSubQuery)

		// SELECT * FROM <table> WHERE id IN (SELECT <table>_id FROM fieldInfo.fieldForeignKey WHERE <other_table>_id IN (SELECT id FROM <other_table> WHERE givenFilter))
		return resultDB.Where("id IN (?)", subQuery), nil
	}

	return nil, fmt.Errorf("relationType '%v' unknown", fieldInfo.relationType)
}
