package deepgorm

import (
	"github.com/google/uuid"
	"github.com/ing-bank/gormtestutil"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"
	"testing"
)

func TestNew_DeepLikeEnablesFeature(t *testing.T) {
	t.Parallel()
	// Act
	plugin := New(Wildcards())

	// Assert
	assert.True(t, plugin.wildcards)
}
func TestDeepGorm_Name_ReturnsExpectedName(t *testing.T) {
	t.Parallel()
	// Arrange
	plugin := New()

	// Act
	result := plugin.Name()

	// Assert
	assert.Equal(t, "deepgorm", result)
}

func TestDeepGorm_Initialize_RegistersCallback(t *testing.T) {
	t.Parallel()
	// Arrange
	db := gormtestutil.NewMemoryDatabase(t, gormtestutil.WithName(t.Name()))
	plugin := New()

	// Act
	err := plugin.Initialize(db)

	// Assert
	assert.Nil(t, err)
	assert.NotNil(t, db.Callback().Query().Get("deepgorm:query"))
}

type ObjectB struct {
	ID   uuid.UUID
	Name string

	ObjectA   *ObjectA `gorm:"foreignKey:ObjectAID"`
	ObjectAID uuid.UUID
}

type ObjectA struct {
	ID   uuid.UUID
	Name string

	ObjectBs []ObjectB `gorm:"foreignKey:ObjectAID"`
}

func TestDeepGorm_Initialize_TriggersFilteringCorrectly(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		filter   map[string]any
		options  []Option
		existing []ObjectA
		expected []ObjectA
	}{
		"nothing": {
			expected: []ObjectA{},
		},
		"no filter": {
			filter: map[string]any{},
			existing: []ObjectA{
				{ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688")},
				{ID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460")},
				{ID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d")},
			},
			expected: []ObjectA{
				{ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"), ObjectBs: []ObjectB{}},
				{ID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"), ObjectBs: []ObjectB{}},
				{ID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"), ObjectBs: []ObjectB{}},
			},
		},
		"simple filter": {
			filter: map[string]any{
				"id": uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
			},
			existing: []ObjectA{
				{ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688")},
				{ID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460")},
				{ID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d")},
			},
			expected: []ObjectA{
				{ID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"), ObjectBs: []ObjectB{}},
			},
		},
		"deep filter": {
			filter: map[string]any{
				"object_bs": map[string]any{
					"name": "abc",
				},
			},
			existing: []ObjectA{
				{
					ID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
							Name: "def",
						},
					},
				},
				{
					ID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("83aaf47d-a167-4a49-8b7c-3516ced56e8a"),
							Name: "abc",
						},
					},
				},
				{
					ID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("3b35e207-c544-424e-b029-be31d5fe8bad"),
							Name: "abc",
						},
					},
				},
			},
			expected: []ObjectA{
				{
					ID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
					ObjectBs: []ObjectB{
						{
							ID:        uuid.MustParse("83aaf47d-a167-4a49-8b7c-3516ced56e8a"),
							Name:      "abc",
							ObjectAID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
						},
					},
				},
				{
					ID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
					ObjectBs: []ObjectB{
						{
							ID:        uuid.MustParse("3b35e207-c544-424e-b029-be31d5fe8bad"),
							Name:      "abc",
							ObjectAID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
						},
					},
				},
			},
		},
		"multi filter": {
			filter: map[string]any{
				"name": "ghi",
				"object_bs": map[string]any{
					"name": "def",
				},
			},
			existing: []ObjectA{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
					Name: "ghi",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
							Name: "def",
						},
					},
				},
				{
					ID:   uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
					Name: "nope",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("83aaf47d-a167-4a49-8b7c-3516ced56e8a"),
							Name: "abc",
						},
					},
				},
				{
					ID:   uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
					Name: "Maybe",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("3b35e207-c544-424e-b029-be31d5fe8bad"),
							Name: "abc",
						},
					},
				},
			},
			expected: []ObjectA{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
					Name: "ghi",
					ObjectBs: []ObjectB{
						{
							ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
							Name:      "def",
							ObjectAID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
						},
					},
				},
			},
		},
		"multi filter with wildcards": {
			filter: map[string]any{
				"name": "*e*",
				"object_bs": map[string]any{
					"name": "abc",
				},
			},
			existing: []ObjectA{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
					Name: "ghi",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
							Name: "def",
						},
					},
				},
				{
					ID:   uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
					Name: "nope",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("3b35e207-c544-424e-b029-be31d5fe8bad"),
							Name: "abc",
						},
					},
				},
				{
					ID:   uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
					Name: "Maybe",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("8b853b20-3066-43e9-8206-46fd92f82b4e"),
							Name: "abc",
						},
					},
				},
			},
			expected: []ObjectA{
				{
					ID:   uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
					Name: "nope",
					ObjectBs: []ObjectB{
						{
							ID:        uuid.MustParse("3b35e207-c544-424e-b029-be31d5fe8bad"),
							Name:      "abc",
							ObjectAID: uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
						},
					},
				},
				{
					ID:   uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
					Name: "Maybe",
					ObjectBs: []ObjectB{
						{
							ID:        uuid.MustParse("8b853b20-3066-43e9-8206-46fd92f82b4e"),
							Name:      "abc",
							ObjectAID: uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
						},
					},
				},
			},
			options: []Option{Wildcards()},
		},
		"deep wildcard filter": {
			filter: map[string]any{
				"object_bs": map[string]any{
					"name": "*e*",
				},
			},
			existing: []ObjectA{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
					Name: "ghi",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
							Name: "def",
						},
					},
				},
				{
					ID:   uuid.MustParse("3415d786-bc03-4543-aa3c-5ec9e55aa460"),
					Name: "nope",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("3b35e207-c544-424e-b029-be31d5fe8bad"),
							Name: "abc",
						},
					},
				},
				{
					ID:   uuid.MustParse("383e9a9b-ef95-421d-a89e-60f0344ee29d"),
					Name: "Maybe",
					ObjectBs: []ObjectB{
						{
							ID:   uuid.MustParse("8b853b20-3066-43e9-8206-46fd92f82b4e"),
							Name: "abc",
						},
					},
				},
			},
			expected: []ObjectA{
				{
					ID:   uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
					Name: "ghi",
					ObjectBs: []ObjectB{
						{
							ID:        uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
							Name:      "def",
							ObjectAID: uuid.MustParse("59aa5a8f-c5de-44fa-9355-080650481688"),
						},
					},
				},
			},
			options: []Option{Wildcards()},
		},
	}

	for name, testData := range tests {
		testData := testData
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			db := gormtestutil.NewMemoryDatabase(t, gormtestutil.WithName(t.Name()))
			_ = db.AutoMigrate(&ObjectA{}, &ObjectB{})
			plugin := New(testData.options...)

			if err := db.CreateInBatches(testData.existing, 10).Error; err != nil {
				t.Error(err)
				t.FailNow()
			}

			// Act
			err := db.Use(plugin)

			// Assert
			assert.Nil(t, err)

			var actual []ObjectA
			err = db.Where(testData.filter).Preload(clause.Associations).Find(&actual).Error
			assert.Nil(t, err)

			assert.Equal(t, testData.expected, actual)
		})
	}
}
