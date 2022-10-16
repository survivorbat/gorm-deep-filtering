package deepgorm

import (
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"
)

// Functions

func resetCache() {
	cacheDatabaseMap = sync.Map{}
}

func newDatabase(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Make sure SQLite uses foreign keys properly, without this it will ignore
	// any errors
	db.Exec("PRAGMA foreign_keys = ON;")

	return db
}

// Mocks

type MockModel struct {
	ID         uuid.UUID
	Name       string
	Metadata   *Metadata `gorm:"foreignKey:MetadataID"`
	MetadataID *uuid.UUID
}

type Metadata struct {
	ID          uuid.UUID
	Name        string
	MockModelID uuid.UUID
	MockModel   *MockModel `gorm:"foreignKey:MockModelID"`
}

type ManyA struct {
	ID     uuid.UUID
	A      string
	ManyBs []*ManyB `gorm:"many2many:a_b"`
}

type ManyB struct {
	ID     uuid.UUID
	B      string
	ManyAs []*ManyA `gorm:"many2many:a_b"`
}

// Tests

func TestGetSpecificStructTagValue_ReturnsExpectedValue(t *testing.T) {
	t.Parallel()
	type TestStruct struct {
		First  string `test:"hello:world;other:one"`
		Second int    `other:"key:value;other,somebody" test:"too" third:"yes:no"`
	}

	tests := map[string]struct {
		key           string
		tag           string
		field         reflect.StructField
		expectedValue string
	}{
		"first": {
			key:           "hello",
			tag:           "test",
			field:         reflect.TypeOf(TestStruct{}).Field(0),
			expectedValue: "world",
		},
		"second": {
			key:           "key",
			tag:           "other",
			field:         reflect.TypeOf(TestStruct{}).Field(1),
			expectedValue: "value",
		},
		"third": {
			key:           "yes",
			tag:           "third",
			field:         reflect.TypeOf(TestStruct{}).Field(1),
			expectedValue: "no",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act
			result, err := getSpecificStructTagValue(testData.field, testData.tag, testData.key)

			// Assert
			assert.Nil(t, err)

			if assert.NotEmpty(t, result) {
				assert.Equal(t, testData.expectedValue, result)
			}
		})
	}
}

func TestGetSpecificStructTagValue_ReturnsErrorOnNoTag(t *testing.T) {
	t.Parallel()
	type TestStruct struct {
		First  string
		Second int `test:"too"`
	}

	tests := map[string]struct {
		key   string
		tag   string
		field reflect.StructField
	}{
		"first": {
			key:   "hello",
			tag:   "test",
			field: reflect.TypeOf(TestStruct{}).Field(0),
		},
		"second": {
			key:   "key",
			tag:   "other",
			field: reflect.TypeOf(TestStruct{}).Field(1),
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act
			result, err := getSpecificStructTagValue(testData.field, testData.tag, testData.key)

			// Assert
			assert.Empty(t, result)

			if assert.NotNil(t, err) {
				expectedError := fmt.Sprintf("tag '%v' not found in field '%v'", testData.tag, testData.field.Name)
				assert.Equal(t, expectedError, err.Error())
			}
		})
	}
}

func TestGetSpecificStructTagValue_ReturnsErrorOnNoKey(t *testing.T) {
	t.Parallel()
	type TestStruct struct {
		First  string `test:"one:two"`
		Second int    `test:"three:four"`
	}

	tests := map[string]struct {
		key   string
		tag   string
		field reflect.StructField
	}{
		"first": {
			key:   "hello",
			tag:   "test",
			field: reflect.TypeOf(TestStruct{}).Field(0),
		},
		"second": {
			key:   "otherkey",
			tag:   "test",
			field: reflect.TypeOf(TestStruct{}).Field(1),
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act
			result, err := getSpecificStructTagValue(testData.field, testData.tag, testData.key)

			// Assert
			assert.Empty(t, result)

			if assert.NotNil(t, err) {
				expectedError := fmt.Sprintf("key '%v' in tag '%v' not found", testData.key, testData.tag)
				assert.Equal(t, expectedError, err.Error())
			}
		})
	}
}

func TestGetDatabaseFieldsOfType_DoesNotReturnSimpleTypes(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct struct {
		//nolint
		Name string
		//nolint
		Occupation string
	}

	resetCache()
	expectedResult := map[string]*nestedType{}

	// Act
	result := getDatabaseFieldsOfType(SimpleStruct{})

	// Assert
	assert.Equal(t, expectedResult, result)
}

func TestGetDatabaseFieldsOfType_ReturnsStructTypeFields(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct struct {
		Name       string
		Occupation string
	}

	type TypeWithStruct struct {
		//nolint
		nestedStruct SimpleStruct `gorm:"foreignKey:nestedStructRef"`
		//nolint
		nestedStructRef int
	}

	resetCache()

	// Act
	result := getDatabaseFieldsOfType(TypeWithStruct{})

	// Assert
	assert.Len(t, result, 1)

	// Check if expected 'nestedStruct' exists
	if assert.NotNil(t, result["nested_struct"]) {
		// Check if it's a SimpleStruct
		assert.Equal(t, &SimpleStruct{}, result["nested_struct"].fieldStructInstance)
		assert.Equal(t, "nested_struct_ref", result["nested_struct"].fieldForeignKey)
		assert.Equal(t, "oneToMany", result["nested_struct"].relationType)
	}
}

func TestGetDatabaseFieldsOfType_ReturnsStructTypeOfSliceFields(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct struct {
		Name              *string
		Occupation        *string
		TypeWithStructRef int
	}

	//nolint
	type SimpleStructs []*SimpleStruct

	type TypeWithStruct struct {
		//nolint
		nestedStruct *SimpleStructs `gorm:"foreignKey:TypeWithStructRef"`
	}

	resetCache()

	// Act
	result := getDatabaseFieldsOfType(TypeWithStruct{})

	// Assert
	assert.Len(t, result, 1)

	// Check if expected 'nestedStruct' exists
	if assert.NotNil(t, result["nested_struct"]) {
		// Check if it's a SimpleStruct
		assert.Equal(t, &SimpleStruct{}, result["nested_struct"].fieldStructInstance)
		assert.Equal(t, "type_with_struct_ref", result["nested_struct"].fieldForeignKey)
		assert.Equal(t, "manyToOne", result["nested_struct"].relationType)
	}
}

func TestGetDatabaseFieldsOfType_ReturnsStructTypeFieldsOnConsecutiveCalls(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct struct {
		Name       string
		Occupation string
	}

	type TypeWithStruct struct {
		//nolint
		nestedStruct SimpleStruct `gorm:"foreignKey:nestedStructRef"`
		//nolint
		nestedStructRef int
	}

	resetCache()

	_ = getDatabaseFieldsOfType(TypeWithStruct{})

	// Act
	result := getDatabaseFieldsOfType(TypeWithStruct{})

	// Assert
	assert.Len(t, result, 1)

	if assert.NotNil(t, result["nested_struct"]) {
		assert.EqualValues(t, &SimpleStruct{}, result["nested_struct"].fieldStructInstance)

		assert.Equal(t, "nested_struct_ref", result["nested_struct"].fieldForeignKey)
		assert.Equal(t, "oneToMany", result["nested_struct"].relationType)
	}
}

func TestEnsureConcrete_TurnsTypeTestAIntoValue(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestA struct{}
	reflectPointer := reflect.TypeOf(&TestA{})

	// Act
	result := ensureConcrete(reflectPointer)

	// Assert
	reflectValue := reflect.TypeOf(TestA{})

	assert.Equal(t, reflectValue, result)
}

func TestEnsureConcrete_TurnsTypeTestBIntoValue(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestB struct{}
	reflectPointer := reflect.TypeOf(&TestB{})

	// Act
	result := ensureConcrete(reflectPointer)

	// Assert
	reflectValue := reflect.TypeOf(TestB{})

	assert.Equal(t, reflectValue, result)
}

func TestEnsureConcrete_TurnsTypeTestAIntoValueWithMultiplePointers(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestA struct{}

	first := &TestA{}
	second := &first
	third := &second

	reflectPointer := reflect.TypeOf(&third)

	// Act
	result := ensureConcrete(reflectPointer)

	// Assert
	reflectValue := reflect.TypeOf(TestA{})

	assert.Equal(t, reflectValue, result)
}

func TestEnsureConcrete_LeavesValueOfTypeTestAAlone(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestA struct{}
	reflectValue := reflect.TypeOf(TestA{})

	// Act
	result := ensureConcrete(reflectValue)

	// Assert
	assert.Equal(t, reflectValue, result)
}

func TestEnsureConcrete_LeavesValueOfTypeTestBAlone(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestB struct{}
	reflectValue := reflect.TypeOf(TestB{})

	// Act
	result := ensureConcrete(reflectValue)

	// Assert
	assert.Equal(t, reflectValue, result)
}

func TestEnsureNotASlice_LeavesValueOfTypeTestAAlone(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestA struct{}
	reflectValue := reflect.TypeOf([]*TestA{})

	// Act
	result := ensureNotASlice(reflectValue)

	// Assert
	expected := reflect.TypeOf(TestA{})
	assert.Equal(t, expected, result)
}

func TestEnsureNotASlice_LeavesValueOfTypeTestBAlone(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestB struct{}
	reflectValue := reflect.TypeOf(TestB{})

	// Act
	result := ensureNotASlice(reflectValue)

	// Assert
	assert.Equal(t, reflectValue, result)
}

func TestEnsureNotASlice_ReturnsExpectedType(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestB struct{}
	reflectValue := reflect.TypeOf([]TestB{})

	// Act
	result := ensureNotASlice(reflectValue)

	// Assert
	expectedReflect := reflect.TypeOf(TestB{})
	assert.Equal(t, expectedReflect, result)
}

func TestEnsureNotASlice_ReturnsExpectedTypeOnDeepSlice(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestB struct{}
	reflectValue := reflect.TypeOf([][][][][][][][][][]TestB{})

	// Act
	result := ensureNotASlice(reflectValue)

	// Assert
	expectedReflect := reflect.TypeOf(TestB{})
	assert.Equal(t, expectedReflect, result)
}

func TestEnsureNotASlice_ReturnsExpectedTypeOnDeepSliceAndPointers(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestB struct{}
	reflectValue := reflect.TypeOf([]*[][]*[]*[][]*[]*[][]*[]*TestB{})

	// Act
	result := ensureNotASlice(reflectValue)

	// Assert
	expectedReflect := reflect.TypeOf(TestB{})
	assert.Equal(t, expectedReflect, result)
}

func TestGetInstanceAndValueTypeInfoOfField_ReturnsExpectedStructOnStruct(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestStruct struct{}
	input := reflect.TypeOf(TestStruct{})

	// Act
	result, relation := getInstanceAndRelationOfField(input)

	// Assert
	assert.Equal(t, &TestStruct{}, result)
	assert.Equal(t, "oneToMany", relation)
}

func TestGetInstanceAndValueTypeInfoOfField_ReturnsExpectedStructOnSlice(t *testing.T) {
	t.Parallel()
	// Arrange
	type TestStruct struct{}
	input := reflect.TypeOf([]TestStruct{})

	// Act
	result, relation := getInstanceAndRelationOfField(input)

	// Assert
	assert.Equal(t, &TestStruct{}, result)
	assert.Equal(t, "manyToOne", relation)
}

func TestGetInstanceAndValueTypeInfoOfField_ReturnsNilOnNonStructUnknownType(t *testing.T) {
	t.Parallel()
	// Arrange
	input := reflect.TypeOf(0)

	// Act
	result, relation := getInstanceAndRelationOfField(input)

	// Assert
	assert.Equal(t, nil, result)
	assert.Equal(t, "", relation)
}

func TestGetNestedType_ReturnsExpectedTypeInfoOnOneToMany(t *testing.T) {
	t.Parallel()
	type NestedStruct struct{}

	type TestStruct struct {
		A *TestStruct   `gorm:"foreignKey:test"`
		B *NestedStruct `gorm:"foreignKey:other"`
	}

	tests := map[string]struct {
		inputType          reflect.Type
		field              reflect.StructField
		expectedForeignKey string
		expected           any
	}{
		"first": {
			expected:           &TestStruct{},
			inputType:          reflect.TypeOf(TestStruct{}),
			field:              reflect.TypeOf(TestStruct{}).Field(0),
			expectedForeignKey: "test",
		},
		"second": {
			expected:           &NestedStruct{},
			inputType:          reflect.TypeOf(TestStruct{}),
			field:              reflect.TypeOf(TestStruct{}).Field(1),
			expectedForeignKey: "other",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act
			result, err := getNestedType(testData.field, testData.inputType)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, result) {
				assert.Equal(t, "oneToMany", result.relationType)
				assert.Equal(t, testData.expectedForeignKey, result.fieldForeignKey)
				assert.Equal(t, testData.expected, result.fieldStructInstance)

				assert.Equal(t, nil, result.destinationManyToManyStructInstance)
				assert.Equal(t, "", result.destinationManyToManyForeignKey)
				assert.Equal(t, "", result.manyToManyTable)
			}
		})
	}
}

func TestGetNestedType_ReturnsExpectedTypeInfoOnManyToOne(t *testing.T) {
	t.Parallel()
	type NestedStruct struct{}

	type TestStruct struct {
		A *[]TestStruct   `gorm:"foreignKey:test"`
		B *[]NestedStruct `gorm:"foreignKey:other"`
	}

	tests := map[string]struct {
		inputType          reflect.Type
		field              reflect.StructField
		expectedForeignKey string
		expected           any
	}{
		"first": {
			expected:           &TestStruct{},
			inputType:          reflect.TypeOf(TestStruct{}),
			field:              reflect.TypeOf(TestStruct{}).Field(0),
			expectedForeignKey: "test",
		},
		"second": {
			expected:           &NestedStruct{},
			inputType:          reflect.TypeOf(TestStruct{}),
			field:              reflect.TypeOf(TestStruct{}).Field(1),
			expectedForeignKey: "other",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act
			result, err := getNestedType(testData.field, nil)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, result) {
				assert.Equal(t, "manyToOne", result.relationType)
				assert.Equal(t, testData.expectedForeignKey, result.fieldForeignKey)
				assert.Equal(t, testData.expected, result.fieldStructInstance)

				assert.Equal(t, nil, result.destinationManyToManyStructInstance)
				assert.Equal(t, "", result.destinationManyToManyForeignKey)
				assert.Equal(t, "", result.manyToManyTable)
			}
		})
	}
}

func TestGetNestedType_ReturnsExpectedTypeInfoOnManyToMany(t *testing.T) {
	t.Parallel()
	// Arrange
	inputType := reflect.TypeOf(ManyA{})
	inputField := inputType.Field(2)

	// This is what ManyA should return
	expected := &nestedType{
		fieldStructInstance:                 &ManyB{},
		fieldForeignKey:                     "many_b_id",
		relationType:                        "manyToMany",
		manyToManyTable:                     "a_b",
		destinationManyToManyForeignKey:     "many_a_id",
		destinationManyToManyStructInstance: &ManyB{},
	}

	// Act
	result, err := getNestedType(inputField, inputType)

	// Assert
	assert.Nil(t, err)

	if assert.NotNil(t, result) {
		assert.EqualValues(t, expected, result)
	}
}

func TestGetNestedType_ReturnsErrorOnNoForeignKeys(t *testing.T) {
	t.Parallel()
	type NestedStruct struct{}

	type TestStruct struct {
		A *[]TestStruct `gorm:""`
		B *[]NestedStruct
	}

	tests := map[string]struct {
		field reflect.StructField
	}{
		"first": {
			field: reflect.TypeOf(TestStruct{}).Field(0),
		},
		"second": {
			field: reflect.TypeOf(TestStruct{}).Field(1),
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act
			result, err := getNestedType(testData.field, nil)

			// Assert
			assert.Nil(t, result)

			if assert.NotNil(t, err) {
				expected := fmt.Sprintf("no 'foreignKey:...' or 'many2many:...' found in field %v", testData.field.Name)
				assert.Equal(t, expected, err.Error())
			}
		})
	}
}

func TestAddDeepFilters_ReturnsErrorOnUnknownFieldInformation(t *testing.T) {
	t.Parallel()
	type SimpleStruct struct {
		Name       string
		Occupation string
	}

	tests := map[string]struct {
		records   []*SimpleStruct
		filterMap map[string]any
		fieldName string
	}{
		"first": {
			records: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			filterMap: map[string]any{
				"probation": map[string]any{},
			},
			fieldName: "probation",
		},
		"second": {
			records: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Roy",
				},
			},
			filterMap: map[string]any{
				"does_not_exist": map[string]any{},
			},
			fieldName: "does_not_exist",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&SimpleStruct{})

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, SimpleStruct{}, testData.filterMap)

			// Assert
			assert.Nil(t, query)

			if assert.NotNil(t, err) {
				expectedError := fmt.Sprintf("field '%v' does not exist", testData.fieldName)
				assert.Equal(t, expectedError, err.Error())
			}
		})
	}
}

func TestAddDeepFilters_AddsSimpleFilters(t *testing.T) {
	t.Parallel()
	type SimpleStruct struct {
		Name       string
		Occupation string
	}

	tests := map[string]struct {
		records   []*SimpleStruct
		expected  []*SimpleStruct
		filterMap map[string]any
	}{
		"first": {
			records: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			expected: []*SimpleStruct{
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			filterMap: map[string]any{
				"occupation": "Ops",
			},
		},
		"second": {
			records: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
				{
					Occupation: "Ops",
					Name:       "Roy",
				},
			},
			expected: []*SimpleStruct{
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
				{
					Occupation: "Ops",
					Name:       "Roy",
				},
			},
			filterMap: map[string]any{
				"occupation": "Ops",
			},
		},
		"third": {
			records: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			expected: []*SimpleStruct{
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			filterMap: map[string]any{
				"occupation": "Ops",
				"name":       "Jennifer",
			},
		},
		"fourth": {
			records: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			expected: []*SimpleStruct{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			filterMap: map[string]any{
				"occupation": []string{"Ops", "Dev"},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&SimpleStruct{})

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, SimpleStruct{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []*SimpleStruct
				query.Preload(clause.Associations).Find(&result)

				assert.EqualValues(t, result, testData.expected)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithOneToMany(t *testing.T) {
	t.Parallel()
	type NestedStruct struct {
		ID         uuid.UUID
		Name       string
		Occupation string
	}

	type ComplexStruct struct {
		ID        uuid.UUID
		Value     int
		Nested    *NestedStruct `gorm:"foreignKey:NestedRef"`
		NestedRef uuid.UUID
	}

	tests := map[string]struct {
		records   []*ComplexStruct
		expected  []ComplexStruct
		filterMap map[string]any
	}{
		"looking for 1 katherina": {
			records: []*ComplexStruct{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Johan",
						Occupation: "Dev",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{

						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Katherina",
						Occupation: "Dev",
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Katherina",
						Occupation: "Dev",
					},
				},
			},
			filterMap: map[string]any{
				"nested": map[string]any{
					"name": "Katherina",
				},
			},
		},
		"looking for 1 katherina and value 11": {
			records: []*ComplexStruct{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Johan",
						Occupation: "Dev",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{

						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Katherina",
						Occupation: "Dev",
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Katherina",
						Occupation: "Dev",
					},
				},
			},
			filterMap: map[string]any{
				"nested": map[string]any{
					"name": "Katherina",
				},
				"value": 11,
			},
		},
		"looking for 2 vanessas": {
			records: []*ComplexStruct{
				{
					ID:        uuid.MustParse("c98dc9f2-bfa5-4ab5-9cbb-76800e09e512"),
					Value:     4,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Vanessa",
						Occupation: "Ops",
					},
				},
				{
					ID:        uuid.MustParse("2ad6a4fe-e0a4-4791-8f10-df6317cdb8b5"),
					Value:     193,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Vanessa",
						Occupation: "Dev",
					},
				},
				{
					ID:        uuid.MustParse("5cc022ae-43a1-44d8-8ab5-31350e68d0b1"),
					Value:     1593,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c5"), // C
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c5"), // C
						Name:       "Derek",
						Occupation: "Dev",
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:        uuid.MustParse("c98dc9f2-bfa5-4ab5-9cbb-76800e09e512"),
					Value:     4,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Vanessa",
						Occupation: "Ops",
					},
				},
				{
					ID:        uuid.MustParse("2ad6a4fe-e0a4-4791-8f10-df6317cdb8b5"),
					Value:     193,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Vanessa",
						Occupation: "Dev",
					},
				},
			},
			filterMap: map[string]any{
				"nested": map[string]any{
					"name": "Vanessa",
				},
			},
		},
		"looking for both coat and joke": {
			records: []*ComplexStruct{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Coat",
						Occupation: "Product Owner",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     2,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Joke",
						Occupation: "Ops",
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Coat",
						Occupation: "Product Owner",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     2,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Joke",
						Occupation: "Ops",
					},
				},
			},
			filterMap: map[string]any{
				"nested": map[string]any{
					"name": []string{"Joke", "Coat"},
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&ComplexStruct{}, &NestedStruct{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			// Crate records
			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, ComplexStruct{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ComplexStruct
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.EqualValues(t, testData.expected, result)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithManyToOneOnSingleFilter(t *testing.T) {
	t.Parallel()
	type Tag struct {
		ID               uuid.UUID
		Key              string
		Value            string
		ComplexStructRef uuid.UUID
	}

	type Tags []*Tag

	type ComplexStruct struct {
		ID   uuid.UUID
		Name string
		Tags *Tags `gorm:"foreignKey:ComplexStructRef"`
	}

	tests := map[string]struct {
		records   []*ComplexStruct
		expected  []ComplexStruct
		filterMap map[string]any
	}{
		"looking for python": {
			records: []*ComplexStruct{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "type",
							Value:            "interpreted",
						},
					},
				},
				{
					ID:   uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // BObject
					Name: "Go",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "compiled",
						},
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "type",
							Value:            "interpreted",
						},
					},
				},
			},
			filterMap: map[string]any{
				"tags": map[string]any{
					"key":   "type",
					"value": "interpreted",
				},
			},
		},
		"javascript-like": {
			records: []*ComplexStruct{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("451d635a-83f2-47da-b12c-50ec49e45509"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // BObject
					Name: "Javascript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
				{
					ID:   uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // C
					Name: "Ruby",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "interpret",
						},
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("451d635a-83f2-47da-b12c-50ec49e45509"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // BObject
					Name: "Javascript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
			},
			filterMap: map[string]any{
				"tags": map[string]any{
					"key":   "like",
					"value": "javascript",
				},
			},
		},
		"no results :(": {
			records: []*ComplexStruct{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("451d635a-83f2-47da-b12c-50ec49e45509"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // BObject
					Name: "Javascript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
			},
			expected: []ComplexStruct{},
			filterMap: map[string]any{
				"tags": map[string]any{
					"key":   "other",
					"value": "tag",
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&ComplexStruct{}, &Tag{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, ComplexStruct{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ComplexStruct
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.EqualValues(t, testData.expected, result)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithManyToOneOnMultiFilter(t *testing.T) {
	t.Parallel()
	type Tag struct {
		ID               uuid.UUID
		Key              string
		Value            string
		ComplexStructRef uuid.UUID
	}

	type Tags []*Tag

	type ComplexStruct struct {
		ID   uuid.UUID
		Name string
		Tags *Tags `gorm:"foreignKey:ComplexStructRef"`
	}

	tests := map[string]struct {
		records   []*ComplexStruct
		expected  []ComplexStruct
		filterMap []map[string]any
	}{
		"looking for python": {
			records: []*ComplexStruct{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "type",
							Value:            "interpreted",
						},
					},
				},
				{
					ID:   uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // BObject
					Name: "Go",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "compiled",
						},
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "type",
							Value:            "interpreted",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"tags": map[string]any{
						"key":   "type",
						"value": "interpreted",
					},
				},
				{
					"name": "Python",
				},
			},
		},
		"javascript-like and not python-like": {
			records: []*ComplexStruct{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("451d635a-83f2-47da-b12c-50ec49e45509"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "like",
							Value:            "javascript",
						},
						{
							ID:               uuid.MustParse("8977cd8b-ebb8-4119-93d5-cbe605d8f668"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "not-like",
							Value:            "python",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // BObject
					Name: "Javascript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
				{
					ID:   uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // C
					Name: "Ruby",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "interpret",
						},
						{
							ID:               uuid.MustParse("8927cd8b-ebb8-4119-93d5-cbe605d8f668"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "not-like",
							Value:            "python",
						},
					},
				},
			},
			expected: []ComplexStruct{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("451d635a-83f2-47da-b12c-50ec49e45509"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "like",
							Value:            "javascript",
						},
						{
							ID:               uuid.MustParse("8977cd8b-ebb8-4119-93d5-cbe605d8f668"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "not-like",
							Value:            "python",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"tags": map[string]any{
						"key":   "like",
						"value": "javascript",
					},
				},
				{
					"tags": map[string]any{
						"key":   "not-like",
						"value": "python",
					},
				},
			},
		},
		"no results :(": {
			records: []*ComplexStruct{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("451d635a-83f2-47da-b12c-50ec49e45509"),
							ComplexStructRef: uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // BObject
					Name: "Javascript",
					Tags: &Tags{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
			},
			expected: []ComplexStruct{},
			filterMap: []map[string]any{
				{
					"tags": map[string]any{
						"key":   "like",
						"value": "javascript",
					},
				},
				{
					"tags": map[string]any{
						"key":   "not-like",
						"value": "javascript",
					},
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&ComplexStruct{}, &Tag{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, ComplexStruct{}, testData.filterMap...)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ComplexStruct
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.EqualValues(t, testData.expected, result)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithManyToManyOnSingleFilter(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		records   []*ManyA
		expected  []ManyA
		filterMap map[string]any
	}{
		"looking for 1 world": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
				{
					ID: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // BObject
					A:  "Goodbye",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("33cac758-83b2-4f16-8704-06ed33a29f69"),
							B:  "space",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
			},
			filterMap: map[string]any{
				"many_bs": map[string]any{
					"b": "world",
				},
			},
		},
		"looking for 2 worlds": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Next",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("967d53a0-67db-4144-8800-7e3cf5c2ad10"),
							B:  "world",
						},
					},
				},
				{
					ID: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // C
					A:  "Goodbye",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("33cac758-83b2-4f16-8704-06ed33a29f69"),
							B:  "space",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Next",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("967d53a0-67db-4144-8800-7e3cf5c2ad10"),
							B:  "world",
						},
					},
				},
			},
			filterMap: map[string]any{
				"many_bs": map[string]any{
					"b": "world",
				},
			},
		},
		"looking for sand or ruins": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Mars",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "gravel",
						},
						{
							ID: uuid.MustParse("3fc686ac-7847-405e-b569-df46b9ea78fe"),
							B:  "nothing",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Pluto",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "sand",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "ruins",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Pluto",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "sand",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "ruins",
						},
					},
				},
			},
			filterMap: map[string]any{
				"many_bs": map[string]any{
					"b": []string{"sand", "ruins"},
				},
			},
		},
		"looking for chalk that has apples": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Chalk",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "apples",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Board",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "pears",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "bananas",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Chalk",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "apples",
						},
					},
				},
			},
			filterMap: map[string]any{
				"a": "Chalk",
				"many_bs": map[string]any{
					"b": []string{"apples"},
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&ManyA{}, &ManyB{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, ManyA{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ManyA
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.EqualValues(t, testData.expected, result)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithManyToManyOnMultiFilter(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		records   []*ManyA
		expected  []ManyA
		filterMap []map[string]any
	}{
		"looking for 1 world": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
				{
					ID: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // BObject
					A:  "Goodbye",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("33cac758-83b2-4f16-8704-06ed33a29f69"),
							B:  "space",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"many_bs": map[string]any{
						"b": "world",
					},
				},
				{
					"a": "Hello",
				},
			},
		},
		"looking for world and planet": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
						{
							ID: uuid.MustParse("967d53a0-67db-4144-8800-7e3cf5c2ad11"),
							B:  "planet",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Next",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575624e1"),
							B:  "world",
						},
						{
							ID: uuid.MustParse("967d53a0-67db-4144-8800-7e3cf5c2ad10"),
							B:  "planet",
						},
					},
				},
				{
					ID: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"), // C
					A:  "Goodbye",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("33cac758-83b2-4f16-8704-06ed33a29f69"),
							B:  "space",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Hello",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("967d53a0-67db-4144-8800-7e3cf5c2ad11"),
							B:  "planet",
						},
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "world",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Next",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("967d53a0-67db-4144-8800-7e3cf5c2ad10"),
							B:  "planet",
						},
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575624e1"),
							B:  "world",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"many_bs": map[string]any{
						"b": "world",
					},
				},
				{
					"many_bs": map[string]any{
						"b": "planet",
					},
				},
			},
		},
		"looking for sand or ruins": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Mars",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "gravel",
						},
						{
							ID: uuid.MustParse("3fc686ac-7847-405e-b569-df46b9ea78fe"),
							B:  "nothing",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Pluto",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "sand",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "ruins",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Pluto",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "sand",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "ruins",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"many_bs": map[string]any{
						"b": []string{"sand", "ruins"},
					},
				},
				{
					"a": "Pluto",
				},
			},
		},
		"looking for chalk that has apples": {
			records: []*ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Chalk",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "apples",
						},
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b01499"),
							B:  "pears",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Chalk",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "bananas",
						},
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "pears",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e17"),
							B:  "apples",
						},
					},
				},
			},
			expected: []ManyA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					A:  "Chalk",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b01499"),
							B:  "pears",
						},
						{
							ID: uuid.MustParse("9f1baf72-6ca5-4d43-8a01-d845575620e1"),
							B:  "apples",
						},
					},
				},
				{
					ID: uuid.MustParse("eeb25c63-be10-4d88-b256-255e7f022a9c"), // BObject
					A:  "Chalk",
					ManyBs: []*ManyB{
						{
							ID: uuid.MustParse("3e4fc93a-20b0-4716-809a-d81ec4b11499"),
							B:  "pears",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e07"),
							B:  "bananas",
						},
						{
							ID: uuid.MustParse("9b87bfed-6820-4234-8cc7-6772cf036e17"),
							B:  "apples",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"a": "Chalk",
					"many_bs": map[string]any{
						"b": []string{"apples"},
					},
				},
				{
					"many_bs": map[string]any{
						"b": []string{"pears"},
					},
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&ManyA{}, &ManyB{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, ManyA{}, testData.filterMap...)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ManyA
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.Len(t, result, len(testData.expected))

				for index, item := range result {
					assert.Equal(t, testData.expected[index].ID, item.ID)
					assert.Equal(t, testData.expected[index].A, item.A)

					for deepIndex, deepItem := range item.ManyBs {
						assert.Equal(t, testData.expected[index].ManyBs[deepIndex].ID, deepItem.ID)
						assert.Equal(t, testData.expected[index].ManyBs[deepIndex].B, deepItem.B)
					}
				}
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithManyToMany2(t *testing.T) {
	t.Parallel()
	type Tag struct {
		ID    uuid.UUID
		Key   string
		Value string
	}

	type Resource struct {
		ID   uuid.UUID
		Name string
		Tags []*Tag `gorm:"many2many:resource_tags"`
	}

	tests := map[string]struct {
		records   []*Resource
		expected  []Resource
		filterMap map[string]any
	}{
		"looking for 1 resource": {
			records: []*Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "TestResource",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
					},
				},
			},
			expected: []Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "TestResource",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
					},
				},
			},
			filterMap: map[string]any{
				"tags": map[string]any{
					"key":   "tenant",
					"value": "InfraNL",
				},
			},
		},
		"looking for 2 resource(s)": {
			records: []*Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "A",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"), // BObject
					Name: "BObject",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be350090"),
							Key:   "tenant",
							Value: "OutraNL",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-020650481688"), // BObject
					Name: "C",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-847a-d5e7be350090"),
							Key:   "tenant",
							Value: "OutraBE",
						},
					},
				},
			},
			expected: []Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "A",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"), // BObject
					Name: "BObject",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be350090"),
							Key:   "tenant",
							Value: "OutraNL",
						},
					},
				},
			},
			filterMap: map[string]any{
				"tags": map[string]any{
					"key":   "tenant",
					"value": []string{"InfraNL", "OutraNL"},
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&Resource{}, &Tag{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, Resource{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []Resource
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.EqualValues(t, testData.expected, result)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithManyToMany2OnMultiFilter(t *testing.T) {
	t.Parallel()
	type Tag struct {
		ID    uuid.UUID
		Key   string
		Value string
	}

	type Resource struct {
		ID   uuid.UUID
		Name string
		Tags []*Tag `gorm:"many2many:resource_tags"`
	}

	tests := map[string]struct {
		records   []*Resource
		expected  []Resource
		filterMap []map[string]any
	}{
		"looking for 1 resource": {
			records: []*Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "TestResource",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359091"),
							Key:   "pcode",
							Value: "P02012",
						},
					},
				},
			},
			expected: []Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "TestResource",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359091"),
							Key:   "pcode",
							Value: "P02012",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"tags": map[string]any{
						"key":   "tenant",
						"value": "InfraNL",
					},
				},
				{
					"tags": map[string]any{
						"key":   "pcode",
						"value": "P02012",
					},
				},
			},
		},
		"looking for 2 resource(s)": {
			records: []*Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "A",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359091"),
							Key:   "pcode",
							Value: "P02012",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"), // BObject
					Name: "BObject",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be350090"),
							Key:   "tenant",
							Value: "OutraNL",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-736d-421f-897a-d5e7be359091"),
							Key:   "pcode",
							Value: "P02329",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-020650481688"), // BObject
					Name: "C",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-847a-d5e7be350090"),
							Key:   "tenant",
							Value: "OutraBE",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359099"),
							Key:   "pcode",
							Value: "P02329",
						},
					},
				},
			},
			expected: []Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "A",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359090"),
							Key:   "tenant",
							Value: "InfraNL",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be359091"),
							Key:   "pcode",
							Value: "P02012",
						},
					},
				},
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"), // BObject
					Name: "BObject",
					Tags: []*Tag{
						{
							ID:    uuid.MustParse("0e2cdda8-734d-421f-897a-d5e7be350090"),
							Key:   "tenant",
							Value: "OutraNL",
						},
						{
							ID:    uuid.MustParse("0e2cdda8-736d-421f-897a-d5e7be359091"),
							Key:   "pcode",
							Value: "P02329",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"tags": map[string]any{
						"key":   "tenant",
						"value": []string{"InfraNL", "OutraNL"},
					},
				},
				{
					"tags": map[string]any{
						"key":   "pcode",
						"value": []string{"P02012", "P02329"},
					},
				},
			},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			database := newDatabase(t)
			_ = database.AutoMigrate(&Resource{}, &Tag{})

			// Make sure SQLite uses foreign keys properly, without this it will ignore
			// any errors
			database.Exec("PRAGMA foreign_keys = ON;")

			database.CreateInBatches(testData.records, len(testData.records))

			resetCache()

			// Act
			query, err := AddDeepFilters(database, Resource{}, testData.filterMap...)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []Resource
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.EqualValues(t, testData.expected, result)
			}
		})
	}
}
