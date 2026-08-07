package icd11

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileSelectsOnlyPrimaryCategories(t *testing.T) {
	values := []string{"Foundation URI", "Linearization URI", "Code", "BlockId", "Title", "ClassKind", "DepthInKind", "IsResidual", "ChapterNo", "BrowserLink", "isLeaf", "Primary tabulation", "Grouping1", "Grouping2", "Grouping3", "Grouping4", "Grouping5", "CodingNote", "Parent", "Version:2026 Jan 17 - 05:30 UTC", "", "foundation/1", "linear/1", "1A00", "Cholera", "category", "False", "01", "True", "foundation/x", "linear/x", "XS00", "Extension", "category", "False", "X"}
	path := filepath.Join(t.TempDir(), "icd.xlsx")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	ss, _ := z.Create("xl/sharedStrings.xml")
	fmt.Fprint(ss, `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, v := range values {
		fmt.Fprintf(ss, "<si><t>%s</t></si>", v)
	}
	fmt.Fprint(ss, "</sst>")
	sheet, _ := z.Create("xl/worksheets/sheet1.xml")
	fmt.Fprint(sheet, `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	row := func(n int, indexes []int) {
		fmt.Fprintf(sheet, "<row r=\"%d\">", n)
		for i, x := range indexes {
			fmt.Fprintf(sheet, "<c r=\"%c%d\" t=\"s\"><v>%d</v></c>", 'A'+i, n, x)
		}
		fmt.Fprint(sheet, "</row>")
	}
	row(1, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	row(2, []int{21, 22, 23, 20, 24, 25, 20, 26, 27, 20, 28, 28, 20, 20, 20, 20, 20, 20, 20})
	row(3, []int{29, 30, 31, 20, 32, 33, 20, 34, 35, 20, 28, 34, 20, 20, 20, 20, 20, 20, 20})
	fmt.Fprint(sheet, "</sheetData></worksheet>")
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	wb, e := ParseFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if wb.Version != "2026-01-17" || len(wb.Concepts) != 1 || wb.Concepts[0].Code != "1A00" {
		t.Fatalf("unexpected workbook: %#v", wb)
	}
	if strings.HasPrefix(wb.Concepts[0].Display, "-") {
		t.Fatal("display hierarchy marker was not cleaned")
	}
}
