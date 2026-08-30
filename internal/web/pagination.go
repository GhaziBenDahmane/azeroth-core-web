package web

import (
	"net/http"
	"strconv"
	"strings"
)

type pagination struct {
	Page        int  `json:"page"`
	PerPage     int  `json:"perPage"`
	Total       int  `json:"total"`
	TotalPages  int  `json:"totalPages"`
	HasPrevious bool `json:"hasPrevious"`
	HasNext     bool `json:"hasNext"`
}

func likePattern(value string) string {
	return "%" + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "%", "\\%"), "_", "\\_") + "%"
}

func requestPage(r *http.Request, defaultSize, maximumSize int) (int, int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
	if page < 1 {
		page = 1
	}
	if defaultSize < 1 {
		defaultSize = 25
	}
	if maximumSize < defaultSize {
		maximumSize = defaultSize
	}
	if perPage < 1 {
		perPage = defaultSize
	}
	if perPage > maximumSize {
		perPage = maximumSize
	}
	return page, perPage, (page - 1) * perPage
}

func paginationMeta(page, perPage, total int) pagination {
	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	return pagination{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages, HasPrevious: page > 1, HasNext: page < totalPages}
}

func slicePage[T any](values []T, page, perPage int) ([]T, pagination) {
	meta := paginationMeta(page, perPage, len(values))
	page = meta.Page
	start := (page - 1) * perPage
	if start >= len(values) {
		return []T{}, meta
	}
	end := start + perPage
	if end > len(values) {
		end = len(values)
	}
	return values[start:end], meta
}
