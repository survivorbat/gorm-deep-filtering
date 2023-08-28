package deepgorm

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"
)

// Functions

func newDatabase(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Make sure SQLite uses foreign keys properly, without this it will ignore any errors
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

func TestGetDatabaseFieldsOfType_DoesNotReturnSimpleTypes(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct1 struct {
		//nolint
		Name string
		//nolint
		Occupation string
	}
	expectedResult := map[string]*nestedType{}

	naming := newDatabase(t).NamingStrategy
	schemaInfo, _ := schema.Parse(&SimpleStruct1{}, &sync.Map{}, naming)

	// Act
	result := getDatabaseFieldsOfType(nil, schemaInfo)

	// Assert
	assert.Equal(t, expectedResult, result)
}

func TestGetDatabaseFieldsOfType_ReturnsStructTypeFields(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct2 struct {
		ID         int
		Name       string
		Occupation string
	}

	type TypeWithStruct1 struct {
		ID              int
		NestedStruct    SimpleStruct2 `gorm:"foreignKey:NestedStructRef"`
		NestedStructRef int
	}

	naming := newDatabase(t).NamingStrategy

	schemaInfo, _ := schema.Parse(TypeWithStruct1{}, &sync.Map{}, naming)

	// Act
	result := getDatabaseFieldsOfType(naming, schemaInfo)

	// Assert
	assert.Len(t, result, 1)

	// Check if expected 'NestedStruct1' exists
	if assert.NotNil(t, result["nested_struct"]) {
		// Check if it's a SimpleStruct1
		assert.IsType(t, &SimpleStruct2{}, result["nested_struct"].fieldStructInstance)
		assert.Equal(t, "nested_struct_ref", result["nested_struct"].fieldForeignKey)
		assert.Equal(t, "oneToMany", result["nested_struct"].relationType)
	}
}

func TestGetDatabaseFieldsOfType_ReturnsStructTypeOfSliceFields(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct3 struct {
		ID                int
		Name              *string
		Occupation        *string
		TypeWithStructRef int
	}

	type TypeWithStruct2 struct {
		ID           int
		NestedStruct []*SimpleStruct3 `gorm:"foreignKey:TypeWithStructRef"`
	}

	naming := newDatabase(t).NamingStrategy

	schemaInfo, _ := schema.Parse(&TypeWithStruct2{}, &sync.Map{}, naming)

	// Act
	result := getDatabaseFieldsOfType(naming, schemaInfo)

	// Assert
	assert.Len(t, result, 1)

	// Check if expected 'NestedStruct1' exists
	if assert.NotNil(t, result["nested_struct"]) {
		// Check if it's a SimpleStruct1
		assert.IsType(t, &SimpleStruct3{}, result["nested_struct"].fieldStructInstance)
		assert.Equal(t, "type_with_struct_ref", result["nested_struct"].fieldForeignKey)
		assert.Equal(t, "manyToOne", result["nested_struct"].relationType)
	}
}

func TestGetDatabaseFieldsOfType_ReturnsStructTypeFieldsOnConsecutiveCalls(t *testing.T) {
	t.Parallel()
	// Arrange
	type SimpleStruct4 struct {
		Name       string
		Occupation string
	}

	type TypeWithStruct3 struct {
		NestedStruct    SimpleStruct4 `gorm:"foreignKey:NestedStructRef"`
		NestedStructRef int
	}

	naming := newDatabase(t).NamingStrategy
	schemaInfo, _ := schema.Parse(&TypeWithStruct3{}, &sync.Map{}, naming)

	_ = getDatabaseFieldsOfType(naming, schemaInfo)

	// Act
	result := getDatabaseFieldsOfType(naming, schemaInfo)

	// Assert
	assert.Len(t, result, 1)

	if assert.NotNil(t, result["nested_struct"]) {
		assert.IsType(t, &SimpleStruct4{}, result["nested_struct"].fieldStructInstance)

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
	type NestedStruct1 struct {
		ID int
	}

	type TestStruct struct {
		ID int

		TestAID int
		A       *TestStruct `gorm:"foreignKey:TestAID"`

		TestBID int
		B       *NestedStruct1 `gorm:"foreignKey:TestBID"`
	}

	tests := map[string]struct {
		field              string
		expectedForeignKey string
		expected           any
	}{
		"first": {
			expected:           &TestStruct{},
			field:              "A",
			expectedForeignKey: "test_a_id",
		},
		"second": {
			expected:           &NestedStruct1{},
			field:              "B",
			expectedForeignKey: "test_b_id",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			naming := newDatabase(t).NamingStrategy
			schemaInfo, _ := schema.Parse(TestStruct{}, &sync.Map{}, naming)
			field := schemaInfo.FieldsByName[testData.field]

			// Act
			result, err := getNestedType(naming, field, reflect.TypeOf(TestStruct{}))

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, result) {
				assert.Equal(t, "oneToMany", result.relationType)
				assert.Equal(t, testData.expectedForeignKey, result.fieldForeignKey)
				assert.Equal(t, testData.expected, result.fieldStructInstance)

				assert.Equal(t, "", result.destinationManyToManyForeignKey)
				assert.Equal(t, "", result.manyToManyTable)
			}
		})
	}
}

func TestGetNestedType_ReturnsExpectedTypeInfoOnManyToOne(t *testing.T) {
	t.Parallel()
	type NestedStruct2 struct {
		ID  int
		BID int
	}

	type TestStruct struct {
		ID int

		AID int
		A   []TestStruct    `gorm:"foreignKey:AID"`
		B   []NestedStruct2 `gorm:"foreignKey:BID"`
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
			field:              reflect.TypeOf(TestStruct{}).Field(2),
			expectedForeignKey: "a_id",
		},
		"second": {
			expected:           &NestedStruct2{},
			inputType:          reflect.TypeOf(TestStruct{}),
			field:              reflect.TypeOf(TestStruct{}).Field(3),
			expectedForeignKey: "b_id",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			naming := newDatabase(t).NamingStrategy
			schemaInfo, _ := schema.Parse(TestStruct{}, &sync.Map{}, naming)
			field := schemaInfo.FieldsByName[testData.field.Name]

			// Act
			result, err := getNestedType(naming, field, nil)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, result) {
				assert.Equal(t, "manyToOne", result.relationType)
				assert.Equal(t, testData.expectedForeignKey, result.fieldForeignKey)
				assert.Equal(t, testData.expected, result.fieldStructInstance)

				assert.Equal(t, "", result.destinationManyToManyForeignKey)
				assert.Equal(t, "", result.manyToManyTable)
			}
		})
	}
}

func TestGetNestedType_ReturnsExpectedTypeInfoOnManyToMany(t *testing.T) {
	t.Parallel()
	// Arrange
	naming := newDatabase(t).NamingStrategy

	schemaInfo, _ := schema.Parse(ManyA{}, &sync.Map{}, naming)
	field := schemaInfo.FieldsByName["ManyBs"]

	inputType := reflect.TypeOf(ManyA{})

	// This is what ManyA should return
	expected := &nestedType{
		fieldStructInstance:             &ManyB{},
		fieldForeignKey:                 "many_b_id",
		relationType:                    "manyToMany",
		manyToManyTable:                 "a_b",
		destinationManyToManyForeignKey: "many_a_id",
	}

	// Act
	result, err := getNestedType(naming, field, inputType)

	// Assert
	assert.Nil(t, err)

	if assert.NotNil(t, result) {
		assert.EqualValues(t, expected, result)
	}
}

func TestGetNestedType_ReturnsErrorOnNoForeignKeys(t *testing.T) {
	t.Parallel()
	type NestedStruct3 struct{}

	type TestStruct struct {
		A *[]TestStruct `gorm:""`
		B *[]NestedStruct3
	}

	tests := map[string]struct {
		field string
	}{
		"first": {
			field: "A",
		},
		"second": {
			field: "B",
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			naming := newDatabase(t).NamingStrategy
			schemaInfo, _ := schema.Parse(TestStruct{}, &sync.Map{}, naming)
			field := schemaInfo.FieldsByName[testData.field]

			// Act
			result, err := getNestedType(naming, field, nil)

			// Assert
			assert.Nil(t, result)

			if assert.NotNil(t, err) {
				expected := fmt.Sprintf("no 'foreignKey:...' or 'many2many:...' found in field %v", testData.field)
				assert.Equal(t, expected, err.Error())
			}
		})
	}
}

func TestAddDeepFilters_ReturnsErrorOnUnknownFieldInformation(t *testing.T) {
	t.Parallel()
	type SimpleStruct5 struct {
		Name       string
		Occupation string
	}

	tests := map[string]struct {
		records   []*SimpleStruct5
		filterMap map[string]any
		fieldName string
	}{
		"first": {
			records: []*SimpleStruct5{
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
			records: []*SimpleStruct5{
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
			_ = database.AutoMigrate(&SimpleStruct5{})

			database.CreateInBatches(testData.records, len(testData.records))

			// Act
			query, err := AddDeepFilters(database, SimpleStruct5{}, testData.filterMap)

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
	type SimpleStruct6 struct {
		Name       string
		Occupation string
	}

	tests := map[string]struct {
		records   []*SimpleStruct6
		expected  []*SimpleStruct6
		filterMap map[string]any
	}{
		"first": {
			records: []*SimpleStruct6{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			expected: []*SimpleStruct6{
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
			records: []*SimpleStruct6{
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
			expected: []*SimpleStruct6{
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
			records: []*SimpleStruct6{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			expected: []*SimpleStruct6{
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
			records: []*SimpleStruct6{
				{
					Occupation: "Dev",
					Name:       "John",
				},
				{
					Occupation: "Ops",
					Name:       "Jennifer",
				},
			},
			expected: []*SimpleStruct6{
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
			_ = database.AutoMigrate(&SimpleStruct6{})

			database.CreateInBatches(testData.records, len(testData.records))

			// Act
			query, err := AddDeepFilters(database, SimpleStruct6{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []*SimpleStruct6
				query.Preload(clause.Associations).Find(&result)

				assert.EqualValues(t, result, testData.expected)
			}
		})
	}
}

func TestAddDeepFilters_AddsDeepFiltersWithOneToMany(t *testing.T) {
	t.Parallel()
	type NestedStruct4 struct {
		ID         uuid.UUID
		Name       string
		Occupation string
	}

	type ComplexStruct1 struct {
		ID        uuid.UUID
		Value     int
		Nested    *NestedStruct4 `gorm:"foreignKey:NestedRef"`
		NestedRef uuid.UUID
	}

	tests := map[string]struct {
		records   []*ComplexStruct1
		expected  []ComplexStruct1
		filterMap map[string]any
	}{
		"looking for 1 katherina": {
			records: []*ComplexStruct1{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Johan",
						Occupation: "Dev",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{

						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Katherina",
						Occupation: "Dev",
					},
				},
			},
			expected: []ComplexStruct1{
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{
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
			records: []*ComplexStruct1{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Johan",
						Occupation: "Dev",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{

						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Katherina",
						Occupation: "Dev",
					},
				},
			},
			expected: []ComplexStruct1{
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     11,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{
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
			records: []*ComplexStruct1{
				{
					ID:        uuid.MustParse("c98dc9f2-bfa5-4ab5-9cbb-76800e09e512"),
					Value:     4,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Vanessa",
						Occupation: "Ops",
					},
				},
				{
					ID:        uuid.MustParse("2ad6a4fe-e0a4-4791-8f10-df6317cdb8b5"),
					Value:     193,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Vanessa",
						Occupation: "Dev",
					},
				},
				{
					ID:        uuid.MustParse("5cc022ae-43a1-44d8-8ab5-31350e68d0b1"),
					Value:     1593,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c5"), // C
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c5"), // C
						Name:       "Derek",
						Occupation: "Dev",
					},
				},
			},
			expected: []ComplexStruct1{
				{
					ID:        uuid.MustParse("c98dc9f2-bfa5-4ab5-9cbb-76800e09e512"),
					Value:     4,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Vanessa",
						Occupation: "Ops",
					},
				},
				{
					ID:        uuid.MustParse("2ad6a4fe-e0a4-4791-8f10-df6317cdb8b5"),
					Value:     193,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{
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
			records: []*ComplexStruct1{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Coat",
						Occupation: "Product Owner",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     2,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
						Name:       "Joke",
						Occupation: "Ops",
					},
				},
			},
			expected: []ComplexStruct1{
				{
					ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Value:     1,
					NestedRef: uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
					Nested: &NestedStruct4{
						ID:         uuid.MustParse("71766db4-eb17-4457-a85c-8b89af5a319d"), // A
						Name:       "Coat",
						Occupation: "Product Owner",
					},
				},
				{
					ID:        uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
					Value:     2,
					NestedRef: uuid.MustParse("4604bb79-ee05-4a09-b874-c3af8964d8c4"), // BObject
					Nested: &NestedStruct4{
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
			_ = database.AutoMigrate(&ComplexStruct1{}, &NestedStruct4{})

			// Crate records
			database.CreateInBatches(testData.records, len(testData.records))

			// Act
			query, err := AddDeepFilters(database, ComplexStruct1{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ComplexStruct1
				res := query.Preload(clause.Associations).Find(&result)

				// Handle error
				assert.Nil(t, res.Error)

				assert.Equal(t, testData.expected, result)
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

	type ComplexStruct2 struct {
		ID   uuid.UUID
		Name string
		Tags []*Tag `gorm:"foreignKey:ComplexStructRef"`
	}

	tests := map[string]struct {
		records   []*ComplexStruct2
		expected  []ComplexStruct2
		filterMap map[string]any
	}{
		"looking for python": {
			records: []*ComplexStruct2{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: []*Tag{
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
					Tags: []*Tag{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "compiled",
						},
					},
				},
			},
			expected: []ComplexStruct2{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: []*Tag{
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
			records: []*ComplexStruct2{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: []*Tag{
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
					Tags: []*Tag{
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
					Tags: []*Tag{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "interpret",
						},
					},
				},
			},
			expected: []ComplexStruct2{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: []*Tag{
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
					Tags: []*Tag{
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
			records: []*ComplexStruct2{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: []*Tag{
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
					Tags: []*Tag{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
			},
			expected: []ComplexStruct2{},
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
			_ = database.AutoMigrate(&ComplexStruct2{}, &Tag{})

			database.CreateInBatches(testData.records, len(testData.records))

			// Act
			query, err := AddDeepFilters(database, ComplexStruct2{}, testData.filterMap)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ComplexStruct2
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

	type ComplexStruct3 struct {
		ID   uuid.UUID
		Name string
		Tags []*Tag `gorm:"foreignKey:ComplexStructRef"`
	}

	tests := map[string]struct {
		records   []*ComplexStruct3
		expected  []ComplexStruct3
		filterMap []map[string]any
	}{
		"looking for python": {
			records: []*ComplexStruct3{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: []*Tag{
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
					Tags: []*Tag{
						{
							ID:               uuid.MustParse("17983ba8-2d26-4e36-bb6b-6c5a04b6606e"),
							ComplexStructRef: uuid.MustParse("23292d51-4768-4c41-8475-6d4c9f0c6f69"),
							Key:              "type",
							Value:            "compiled",
						},
					},
				},
			},
			expected: []ComplexStruct3{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"), // A
					Name: "Python",
					Tags: []*Tag{
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
			records: []*ComplexStruct3{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: []*Tag{
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
					Tags: []*Tag{
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
					Tags: []*Tag{
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
			expected: []ComplexStruct3{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: []*Tag{
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
			records: []*ComplexStruct3{
				{
					ID:   uuid.MustParse("411ed385-c1ca-432d-b577-6d6138450264"), // A
					Name: "Typescript",
					Tags: []*Tag{
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
					Tags: []*Tag{
						{
							ID:               uuid.MustParse("1c83a7c9-e95d-4dba-b858-5eb4e34ebcf2"),
							ComplexStructRef: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
							Key:              "like",
							Value:            "javascript",
						},
					},
				},
			},
			expected: []ComplexStruct3{},
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
			_ = database.AutoMigrate(&ComplexStruct3{}, &Tag{})

			database.CreateInBatches(testData.records, len(testData.records))

			// Act
			query, err := AddDeepFilters(database, ComplexStruct3{}, testData.filterMap...)

			// Assert
			assert.Nil(t, err)

			if assert.NotNil(t, query) {
				var result []ComplexStruct3
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

			database.CreateInBatches(testData.records, len(testData.records))

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
			database := newDatabase(t).Debug()
			_ = database.AutoMigrate(&ManyA{}, &ManyB{})

			database.CreateInBatches(testData.records, len(testData.records))

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

			database.CreateInBatches(testData.records, len(testData.records))

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

			database.CreateInBatches(testData.records, len(testData.records))

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

func TestAddDeepFilters_AddsDeepFiltersWithManyToManyCustomFields(t *testing.T) {
	t.Parallel()
	type End struct {
		ID    uuid.UUID `gorm:"column:endId;primaryKey"`
		Value string    `gorm:"column:endValue"`
	}

	type Middle struct {
		ID         uuid.UUID `gorm:"column:middleId;primaryKey"`
		ResourceId uuid.UUID `gorm:"column:resourceIdJ"`
		EndId      uuid.UUID `gorm:"column:endIdJ"`
	}

	type Resource struct {
		ID   uuid.UUID `gorm:"column:resourceId;primaryKey"`
		Name string    `gorm:"column:resourceName"`
		Ends []*End    `gorm:"many2many:middles;foreignKey:resourceId;joinForeignKey:resourceIdJ;References:endId;JoinReferences:endIdJ"`
	}

	tests := map[string]struct {
		records   []*Resource
		expected  []Resource
		filterMap []map[string]any
	}{
		"looking for 1 resource": {
			records: []*Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Name: "TestResource",
					Ends: []*End{
						{
							ID:    uuid.MustParse("c53184d8-e506-49f4-af18-93fb370f6df2"), // A
							Value: "InfraNL",
						},
						{
							ID:    uuid.MustParse("4de16d5f-c10f-4206-b6ce-c14997341113"), // B
							Value: "Blub",
						},
					},
				},
			},
			expected: []Resource{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481687"),
					Name: "TestResource",
					Ends: []*End{
						{
							ID:    uuid.MustParse("c53184d8-e506-49f4-af18-93fb370f6df2"), // A
							Value: "InfraNL",
						},
					},
				},
			},
			filterMap: []map[string]any{
				{
					"ends": map[string]any{
						"value": "InfraNL",
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
			database.NamingStrategy = schema.NamingStrategy{
				SingularTable: true,
				NoLowerCase:   false,
				NameReplacer: strings.NewReplacer(
					"resource_id_j", "resourceIdJ",
					"end_id_j", "endIdJ",
				),
			}
			_ = database.AutoMigrate(&Resource{}, &Middle{}, &End{})

			database.CreateInBatches(testData.records, len(testData.records))

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
