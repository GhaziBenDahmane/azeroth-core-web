package web

import (
	"net/http/httptest"
	"testing"
)

func TestRequestPageBounds(t *testing.T) {
	r := httptest.NewRequest("GET", "/?page=3&perPage=999", nil)
	page, size, offset := requestPage(r, 25, 100)
	if page != 3 || size != 100 || offset != 200 {
		t.Fatalf("got page=%d size=%d offset=%d", page, size, offset)
	}
}

func TestSlicePage(t *testing.T) {
	page, meta := slicePage([]int{1, 2, 3, 4, 5}, 2, 2)
	if len(page) != 2 || page[0] != 3 || !meta.HasPrevious || !meta.HasNext || meta.TotalPages != 3 {
		t.Fatalf("page=%v meta=%+v", page, meta)
	}
}
