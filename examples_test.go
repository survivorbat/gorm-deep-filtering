package deepgorm

import (
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func ExampleNew() {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	_ = db.Use(New())
	_ = db.Use(New(DeepLike()))
}

// Get all ObjectAs that are connected to ObhectB with Id 50
func ExampleAddDeepFilters() {
	type ObjectB struct {
		ID int
	}

	type ObjectA struct {
		ID int

		ObjectB   *ObjectB `gorm:"foreignKey:ObjectBID"`
		ObjectBID int
	}

	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = db.Use(New())

	db.Create(&ObjectA{
		ID: 1,
		ObjectB: &ObjectB{
			ID: 50,
		},
	})

	filters := map[string]any{
		"object_b": map[string]any{
			"id": 50,
		},
	}

	var result ObjectA
	db.Where(filters).Find(&result)
	fmt.Println(result)
}
