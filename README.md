# 🌌 Gorm Deep Filtering Plugin

Ever wanted to filter objects on a deep level using only maps? This plugin allows you to do just that.

```go
package main

func main () {
	filters := map[string]any{
		"name": "abc",
		"related_object": map[string]any{
			"title": "engineer",
		},
	}
}
```

Is automatically turned into a query that looks like this:

```sql
SELECT * FROM employees WHERE related_object_id IN (SELECT id FROM occupations WHERE title = "engineer")
```

## ⬇️ Installation

`go get github.com/survivorbat/gorm-deep-filtering`

## 📋 Usage

```go
package main

import (
    "github.com/survivorbat/gorm-deep-filtering"
)

func main() {
	db, _ := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	
	db.Use(deepgorm.New())
}

```

## 🔭 Plans

Better error handling, logging.
