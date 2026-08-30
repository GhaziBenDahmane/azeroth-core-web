package web

import "testing"

func TestValidateManagedProductArtworkURL(t *testing.T) {
	base := product{
		Name:     "Starter weapon",
		Price:    25,
		Category: "Weapons",
		ItemID:   49623,
		Quantity: 1,
	}

	valid := base
	valid.ImageURL = "https://portal.example/api/media/7/starter-weapon.jpg"
	if err := validateManagedProduct(valid); err != nil {
		t.Fatalf("valid HTTPS product artwork rejected: %v", err)
	}

	unsafe := base
	unsafe.ImageURL = "javascript:alert(1)"
	if err := validateManagedProduct(unsafe); err == nil {
		t.Fatal("unsafe product artwork URL accepted")
	}
}
