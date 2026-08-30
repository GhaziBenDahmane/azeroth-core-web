package web

import "testing"

func TestParseCatalogCSV(t *testing.T) {
	rows, err := parseCatalogCSV("name,price,category,item_id,quantity,items,active,visibility\nStarter Bag,10,Utility,51809,1,,true,all\nRaid Kit,25,PvE,0,0,37719:5;51809:1,true,veteran\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[0].Errors) > 0 || rows[0].Product.ItemID != 51809 || len(rows[1].Product.Items) != 2 || rows[1].Product.Visibility != "veteran" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestParseCatalogCSVReportsInvalidRows(t *testing.T) {
	rows, err := parseCatalogCSV("name,price,category,items\nBroken,nope,Items,bad\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Errors) < 2 {
		t.Fatalf("expected row errors: %#v", rows)
	}
}
