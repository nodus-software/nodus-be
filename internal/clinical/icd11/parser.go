package icd11

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`Version:(\d{4}) ([A-Za-z]{3}) (\d{1,2})`)

type Concept struct {
	Code, Display, SourceTitle, FoundationURI, LinearizationURI, ChapterNo, ParentURI string
	IsLeaf, IsResidual, PrimaryTabulation                                             bool
}

type Workbook struct {
	Version, Title, SourceFile, Checksum string
	ReleasedOn                           time.Time
	TotalRows                            int
	Concepts                             []Concept
}

type sharedStrings struct {
	Items []struct {
		Text []struct {
			Value string `xml:",chardata"`
		} `xml:"t"`
		Runs []struct {
			Text struct {
				Value string `xml:",chardata"`
			} `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}
type worksheet struct {
	Rows []struct {
		Cells []struct {
			Ref   string `xml:"r,attr"`
			Type  string `xml:"t,attr"`
			Value string `xml:"v"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func ParseFile(path string) (*Workbook, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer zr.Close()
	read := func(name string, dst any) error {
		for _, f := range zr.File {
			if f.Name != name {
				continue
			}
			r, e := f.Open()
			if e != nil {
				return e
			}
			defer r.Close()
			return xml.NewDecoder(io.LimitReader(r, 256<<20)).Decode(dst)
		}
		return fmt.Errorf("xlsx entry %s not found", name)
	}
	var ss sharedStrings
	if err = read("xl/sharedStrings.xml", &ss); err != nil {
		return nil, err
	}
	stringsTable := make([]string, len(ss.Items))
	for i, item := range ss.Items {
		var parts []string
		for _, t := range item.Text {
			parts = append(parts, t.Value)
		}
		for _, r := range item.Runs {
			parts = append(parts, r.Text.Value)
		}
		stringsTable[i] = strings.Join(parts, "")
	}
	var sheet worksheet
	if err = read("xl/worksheets/sheet1.xml", &sheet); err != nil {
		return nil, err
	}
	if len(sheet.Rows) < 2 {
		return nil, fmt.Errorf("workbook has no data rows")
	}
	row := func(cells []struct {
		Ref   string `xml:"r,attr"`
		Type  string `xml:"t,attr"`
		Value string `xml:"v"`
	}) (map[string]string, error) {
		out := map[string]string{}
		for _, c := range cells {
			col := strings.TrimRight(c.Ref, "0123456789")
			value := c.Value
			if c.Type == "s" {
				var index int
				if _, e := fmt.Sscanf(value, "%d", &index); e != nil || index < 0 || index >= len(stringsTable) {
					return nil, fmt.Errorf("invalid shared string in %s", c.Ref)
				}
				value = stringsTable[index]
			}
			out[col] = value
		}
		return out, nil
	}
	headers, err := row(sheet.Rows[0].Cells)
	if err != nil {
		return nil, err
	}
	want := map[string]string{"A": "Foundation URI", "B": "Linearization URI", "C": "Code", "E": "Title", "F": "ClassKind", "H": "IsResidual", "I": "ChapterNo", "K": "isLeaf", "L": "Primary tabulation", "S": "Parent"}
	for col, name := range want {
		if headers[col] != name {
			return nil, fmt.Errorf("unexpected %s header: want %q, got %q", col, name, headers[col])
		}
	}
	versionCell := headers["T"]
	m := versionPattern.FindStringSubmatch(versionCell)
	if len(m) != 4 {
		return nil, fmt.Errorf("invalid release marker %q", versionCell)
	}
	released, err := time.Parse("2006 Jan 2", strings.Join(m[1:], " "))
	if err != nil {
		return nil, err
	}
	wb := &Workbook{Version: released.Format("2006-01-02"), Title: "ICD-11 MMS English (" + released.Format("2006-01-02") + ")", SourceFile: filepath.Base(path), Checksum: hex.EncodeToString(sum[:]), ReleasedOn: released, TotalRows: len(sheet.Rows) - 1}
	seenCode, seenURI := map[string]bool{}, map[string]bool{}
	for i, raw := range sheet.Rows[1:] {
		x, e := row(raw.Cells)
		if e != nil {
			return nil, fmt.Errorf("row %d: %w", i+2, e)
		}
		if x["F"] != "category" || x["L"] != "True" {
			continue
		}
		if x["C"] == "" || x["B"] == "" || x["E"] == "" || x["I"] == "" {
			return nil, fmt.Errorf("row %d: primary category is missing required data", i+2)
		}
		if seenCode[x["C"]] || seenURI[x["B"]] {
			return nil, fmt.Errorf("row %d: duplicate code or URI", i+2)
		}
		seenCode[x["C"]], seenURI[x["B"]] = true, true
		wb.Concepts = append(wb.Concepts, Concept{Code: x["C"], Display: strings.TrimSpace(strings.TrimLeft(x["E"], "- ")), SourceTitle: x["E"], FoundationURI: x["A"], LinearizationURI: x["B"], ChapterNo: x["I"], ParentURI: x["S"], IsLeaf: x["K"] == "True", IsResidual: x["H"] == "True", PrimaryTabulation: true})
	}
	if len(wb.Concepts) == 0 {
		return nil, fmt.Errorf("workbook contains no primary MMS categories")
	}
	return wb, nil
}
