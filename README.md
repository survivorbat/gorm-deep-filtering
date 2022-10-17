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

Check out [the examples](./examples_test.go).

## 🔭 Plans

Better error handling, logging.
