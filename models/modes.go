package models

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const dbTag = "select"

var validColumnName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// isSafeColumn 检查列名是否合法，防止 SQL 注入。
// 允许的字符: 小写字母、数字、下划线、点号。
func isSafeColumn(field string) bool {
	validColumn := regexp.MustCompile(`^[a-z0-9_\.]+$`)
	return validColumn.MatchString(field)
}

// escapeColumn 转义 SQL 查询中的列名 (PostgreSQL 版本)。
// - 如果包含 "."，表示带有表别名，例如 "u.name"
// - 否则直接转义整个字段
func escapeColumn(field string) string {
	parts := strings.Split(field, ".")
	if len(parts) == 2 {
		return fmt.Sprintf(`"%s"."%s"`, parts[0], parts[1])
	}
	return fmt.Sprintf(`"%s"`, field)
}

// DealWithWhereSafe 生成 WHERE 子句和参数列表（PostgreSQL 兼容）。
func DealWithWhereSafe(params ...Condition) (string, []interface{}, error) {
	if len(params) == 0 {
		return "", nil, nil
	}

	var (
		conditions []string
		args       []interface{}
		argIndex   = 1 // PostgreSQL 占位符从 $1 开始
	)

	for _, param := range params {
		if !isSafeColumn(param.Field) {
			return "", nil, fmt.Errorf("invalid column name: %s", param.Field)
		}

		symbol := strings.ToUpper(strings.TrimSpace(param.Symbol))
		if symbol == "" {
			symbol = "="
		}

		escapedCol := escapeColumn(param.Field)

		switch symbol {
		case "=", "!=", ">", ">=", "<", "<=", "LIKE":
			conditions = append(conditions, fmt.Sprintf("%s %s $%d", escapedCol, symbol, argIndex))
			args = append(args, param.Value)
			argIndex++

		case "IN":
			valSlice, ok := param.Value.([]interface{})
			if !ok || len(valSlice) == 0 {
				return "", nil, fmt.Errorf("IN requires a non-empty slice for column %s", param.Field)
			}
			placeholders := make([]string, len(valSlice))
			for i, v := range valSlice {
				placeholders[i] = fmt.Sprintf("$%d", argIndex)
				argIndex++
				args = append(args, v)
			}
			conditions = append(conditions, fmt.Sprintf("%s IN (%s)", escapedCol, strings.Join(placeholders, ",")))

		case "BETWEEN":
			valSlice, ok := param.Value.([]interface{})
			if !ok || len(valSlice) != 2 {
				return "", nil, fmt.Errorf("BETWEEN requires exactly 2 values for column %s", param.Field)
			}
			conditions = append(conditions, fmt.Sprintf("%s BETWEEN $%d AND $%d", escapedCol, argIndex, argIndex+1))
			args = append(args, valSlice[0], valSlice[1])
			argIndex += 2

			// PostgreSQL 数组操作符
			// @>  包含：column 数组包含右侧数组/元素
			// <@  被包含：column 被右侧数组包含
			// &&  重叠：column 与右侧数组有交集
		case "@>", "<@", "&&":
			var values []interface{}
			useAny := false
			switch v := param.Value.(type) {
			case []interface{}:
				values = v
			default:
				// 单值：优先用 ANY 避免类型不匹配 (integer[] @> text[])
				useAny = true
			}
			if useAny {
				conditions = append(conditions, fmt.Sprintf("$%d = ANY(%s)", argIndex, escapedCol))
				args = append(args, param.Value)
				argIndex++
			} else if len(values) > 0 {
				arrPlaceholders := make([]string, len(values))
				for i, v := range values {
					arrPlaceholders[i] = fmt.Sprintf("$%d", argIndex)
					args = append(args, v)
					argIndex++
				}
				// 显式类型转换，避免 ARRAY[$n] 被解析为 text[]
				arrType := inferPgArrayType(values)
				conditions = append(conditions, fmt.Sprintf("%s %s ARRAY[%s]::%s", escapedCol, symbol, strings.Join(arrPlaceholders, ","), arrType))
			}

		// ANY 语法糖：检查列数组是否包含某个标量元素
		// 等价于：$n = ANY(column)
		case "ANY":
			conditions = append(conditions, fmt.Sprintf("$%d = ANY(%s)", argIndex, escapedCol))
			args = append(args, param.Value)
			argIndex++

		case "IS NULL":
			conditions = append(conditions, fmt.Sprintf("%s IS NULL", escapedCol))

		case "IS NOT NULL":
			conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", escapedCol))

		default:
			return "", nil, fmt.Errorf("unsupported symbol: %s", symbol)
		}
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")
	return whereClause, args, nil
}

// inferPgArrayType 简单推断数组元素应转换的 PG 类型（默认 int8[]）。
func inferPgArrayType(vals []interface{}) string {
	if len(vals) == 0 {
		return "int8[]"
	}
	switch vals[0].(type) {
	case int, int32, int64:
		return "int8[]"
	case string:
		return "text[]"
	default:
		return "int8[]"
	}
}

func RawFieldNames(in any, postgreSql ...bool) []string {
	out := make([]string, 0)
	v := reflect.ValueOf(in)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var pg bool
	if len(postgreSql) > 0 {
		pg = postgreSql[0]
	}

	if v.Kind() != reflect.Struct {
		panic(fmt.Errorf("RawFieldNames only accepts structs; got %T", v))
	}

	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fi := typ.Field(i)
		tagv := fi.Tag.Get(dbTag)

		if tagv == "-" {
			continue
		}

		// 解析 db:"xxx,option1,option2"
		if strings.Contains(tagv, ",") {
			tagv = strings.TrimSpace(strings.Split(tagv, ",")[0])
		}
		if tagv == "" {
			tagv = fi.Name
		}
		if tagv == "-" {
			continue
		}

		// 处理字段格式化
		if pg {
			out = append(out, tagv)
		} else {
			out = append(out, escapeColumn(tagv))
		}
	}
	return out
}

func GetOrderBy(sorts []Sort, query string) string {
	for i, s := range sorts {
		filed := escapeColumn(s.Filed)
		if i > 0 {
			query += fmt.Sprintf(", %s %s", filed, s.Order)
		} else {
			query += fmt.Sprintf(" order by %s %s", filed, s.Order)
		}
	}
	return query
}

func GetInCondition(ids []int64) (string, []interface{}) {
	if len(ids) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")), args
}

type (
	Total struct {
		Number int64 `db:"number"`
	}
	Pages struct {
		Page int64
		Size int64
	}
	Sort struct {
		Filed string //排序字段
		Order string //顺序：asc 倒叙:desc
	}
	Condition struct {
		Field  string
		Symbol string //符号 可不传，不传默认 =
		Value  interface{}
	}
	ListConditions struct {
		Pages
		Conditions []Condition
		Sorts      []Sort
	}
)
