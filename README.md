# 🌌 Gorm Deep Filtering Plugin

[![Go package](https://github.com/survivorbat/gorm-deep-filtering/actions/workflows/test.yaml/badge.svg)](https://github.com/survivorbat/gorm-deep-filtering/actions/workflows/test.yaml)
![GitHub](https://img.shields.io/github/license/survivorbat/gorm-deep-filtering)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/survivorbat/gorm-deep-filtering)

Ever wanted to filter objects on a deep level using only maps? This plugin allows you to do just that.

There's also an experimental feature that turns wildcard queries (*) into LIKE queries, but this may be changed in
the future.

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
	
	// Adds deep filtering
	db.Use(deepgorm.New())
	
	// Turns strings with wildcards (*) into LIKE queries (EXPERIMENTAL FEATURE)
	db.Use(deepgorm.New(deepgorm.Wildcards()))
}

```

## 🔭 Plans

Better error handling, logging.
