package wpexport

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStoreDate(t *testing.T) {
	got, err := StoreDate("2026-10-04")
	if err != nil || got != "4 octubre, 2026" {
		t.Fatalf("StoreDate = %q, %v", got, err)
	}
	if _, err := StoreDate("04/10/2026"); err == nil {
		t.Fatal("expected an error for a non ISO date")
	}
}

func TestToken(t *testing.T) {
	// md5("key1"), first 16 hex chars (what WP All Export uses).
	if got := Token("key", "1"); len(got) != 16 || got != "c2add694bf942dc7" {
		t.Fatalf("Token = %q", got)
	}
}

func TestSetStoreDate(t *testing.T) {
	rules, _ := json.Marshal([]map[string]any{{"element": "cf_Seleccionar una Fecha", "value": "1 enero, 2020"}})
	form := `<form><input name="wp_all_export_value[1]" value="1 enero, 2020">` +
		`<input type="hidden" name="filter_rules_hierarhy" value="` + strings.ReplaceAll(string(rules), `"`, "&quot;") + `">` +
		`<input type="checkbox" name="off" value="x"><input type="checkbox" name="on" value="y" checked>` +
		`<textarea name="whereclause">AND meta.meta_value = '1 enero, 2020'</textarea>` +
		`<select name="s"><option value="a">A</option><option value="b" selected>B</option></select></form>`
	fields, found := setStoreDate(parseFields(form), "4 octubre, 2026")
	if !found {
		t.Fatal("filter rule not found")
	}
	got := map[string]string{}
	for _, f := range fields {
		got[f.name] = f.value
	}
	if got["wp_all_export_value[1]"] != "4 octubre, 2026" ||
		!strings.Contains(got["filter_rules_hierarhy"], "4 octubre, 2026") ||
		got["whereclause"] != "AND meta.meta_value = '4 octubre, 2026'" ||
		got["s"] != "b" || got["on"] != "y" {
		t.Fatalf("fields = %#v", got)
	}
	if _, ok := got["off"]; ok {
		t.Fatal("unchecked checkbox must not be sent")
	}
}

func TestReadXLSX(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, _ := z.Create(name)
		_, _ = w.Write([]byte(body))
	}
	write("xl/workbook.xml", `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="S" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	write("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/data.xml"/></Relationships>`)
	write("xl/sharedStrings.xml", `<sst><si><t>ID orden</t></si><si><r><t>Pro</t></r><r><t>ductos</t></r></si><si><t>Waffle &amp; café</t></si></sst>`)
	write("xl/worksheets/data.xml", `<worksheet><sheetData>`+
		`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c></row>`+
		`<row r="3"><c r="A3"><v>75758</v></c><c r="C3" t="s"><v>2</v></c><c r="D3" t="inlineStr"><is><t>x</t></is></c></row>`+
		`</sheetData></worksheet>`)
	_ = z.Close()
	rows, err := ReadXLSX(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"ID orden", "", "Productos"}, {}, {"75758", "", "Waffle & café", "x"}}
	if len(rows) != len(want) {
		t.Fatalf("rows = %#v", rows)
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d = %#v, want %#v", i, rows[i], want[i])
		}
	}
}
