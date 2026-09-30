package users

import "fmt"

func ListCacheKey(query string, limit, offset int64) string {
	return fmt.Sprintf("%s|%d|%d", query, limit, offset)
}
