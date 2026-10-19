package usagearchive

import "testing"

func TestNormalizeArchivePageBoundsValues(t *testing.T) {
	page, size := NormalizeArchivePage(0, 0)
	if page != 1 || size != 20 {
		t.Fatalf("NormalizeArchivePage(0,0) = %d,%d", page, size)
	}
	page, size = NormalizeArchivePage(3, 5000)
	if page != 3 || size != 100 {
		t.Fatalf("NormalizeArchivePage(3,5000) = %d,%d", page, size)
	}
}
