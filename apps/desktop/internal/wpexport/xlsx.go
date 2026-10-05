package wpexport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"
)

// ReadXLSX returns the cell values of the first worksheet, row by row.
func ReadXLSX(data []byte) ([][]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, f := range archive.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f, ok := files[name]
		if !ok {
			return nil, errors.New("falta " + name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}

	sheet := firstSheetPath(read)
	var strs []string
	if raw, err := read("xl/sharedStrings.xml"); err == nil {
		if strs, err = sharedStrings(raw); err != nil {
			return nil, err
		}
	}
	raw, err := read(sheet)
	if err != nil {
		return nil, err
	}
	return sheetRows(raw, strs)
}

func firstSheetPath(read func(string) ([]byte, error)) string {
	const fallback = "xl/worksheets/sheet1.xml"
	workbook, err := read("xl/workbook.xml")
	if err != nil {
		return fallback
	}
	var wb struct {
		Sheets []struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if xml.Unmarshal(workbook, &wb) != nil || len(wb.Sheets) == 0 {
		return fallback
	}
	rels, err := read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return fallback
	}
	var rs struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(rels, &rs) != nil {
		return fallback
	}
	for _, r := range rs.Items {
		if r.ID == wb.Sheets[0].ID {
			if strings.HasPrefix(r.Target, "/") {
				return strings.TrimPrefix(r.Target, "/")
			}
			return path.Join("xl", r.Target)
		}
	}
	return fallback
}

// richText is a cell or shared string: plain <t> or rich-text runs <r><t>.
type richText struct {
	T    string `xml:"t"`
	Runs []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (r richText) text() string {
	if len(r.Runs) == 0 {
		return r.T
	}
	var b strings.Builder
	b.WriteString(r.T)
	for _, run := range r.Runs {
		b.WriteString(run.T)
	}
	return b.String()
}

func sharedStrings(raw []byte) ([]string, error) {
	var sst struct {
		Items []richText `xml:"si"`
	}
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, err
	}
	out := make([]string, len(sst.Items))
	for i, item := range sst.Items {
		out[i] = item.text()
	}
	return out, nil
}

func sheetRows(raw []byte, strs []string) ([][]string, error) {
	var ws struct {
		Rows []struct {
			R     int `xml:"r,attr"`
			Cells []struct {
				Ref    string   `xml:"r,attr"`
				Type   string   `xml:"t,attr"`
				Value  string   `xml:"v"`
				Inline richText `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(raw, &ws); err != nil {
		return nil, err
	}
	var rows [][]string
	for i, row := range ws.Rows {
		index := row.R - 1
		if row.R == 0 {
			index = i
		}
		var cells []string
		position := 0
		for _, cell := range row.Cells {
			col := position
			if cell.Ref != "" {
				col = columnIndex(cell.Ref)
			}
			var value string
			switch cell.Type {
			case "s":
				n, err := strconv.Atoi(strings.TrimSpace(cell.Value))
				if err == nil && n >= 0 && n < len(strs) {
					value = strs[n]
				}
			case "inlineStr":
				value = cell.Inline.text()
			default:
				value = cell.Value
			}
			for len(cells) < col {
				cells = append(cells, "")
			}
			if col < len(cells) {
				cells[col] = value
			} else {
				cells = append(cells, value)
			}
			position = col + 1
		}
		for len(rows) < index {
			rows = append(rows, []string{})
		}
		if index < len(rows) {
			rows[index] = cells
		} else {
			rows = append(rows, cells)
		}
	}
	return rows, nil
}

func columnIndex(ref string) int {
	index := 0
	for _, ch := range ref {
		if ch < 'A' || ch > 'Z' {
			break
		}
		index = index*26 + int(ch-'A'+1)
	}
	return index - 1
}
