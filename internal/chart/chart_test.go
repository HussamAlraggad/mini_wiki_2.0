package chart

import (
	"strings"
	"testing"

	"mini-wiki/internal/dataset"
)

func makeTestDS() *dataset.Dataset {
	return &dataset.Dataset{
		Name: "test",
		Columns: []dataset.Column{
			{Name: "name", Kind: dataset.ColumnString},
			{Name: "value", Kind: dataset.ColumnFloat},
			{Name: "count", Kind: dataset.ColumnInteger},
		},
		Rows: []dataset.Row{
			{Data: map[string]interface{}{"name": "A", "value": 10.0, "count": 100}},
			{Data: map[string]interface{}{"name": "B", "value": 20.0, "count": 200}},
			{Data: map[string]interface{}{"name": "C", "value": 30.0, "count": 150}},
		},
		RowCount: 3,
	}
}

func TestBarChart(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Bar,
		ColumnX: "value",
		Width:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Terminal, "10.0") {
		t.Error("bar chart should contain value labels")
	}
}

func TestTrendChart(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Trend,
		ColumnX: "value",
		Width:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Terminal) == 0 {
		t.Error("trend chart should produce output")
	}
}

func TestPieChart(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Pie,
		ColumnX: "value",
		Width:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Terminal, "16.7") {
		t.Error("pie chart should contain percentage labels")
	}
}

func TestScatterChart(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Scatter,
		ColumnX: "value",
		ColumnY: "count",
		Width:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Terminal, "X:") {
		t.Error("scatter chart should have axis labels")
	}
}

func TestHistogram(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Histogram,
		ColumnX: "value",
		Width:   40,
		Buckets: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Terminal) == 0 {
		t.Error("histogram should produce output")
	}
}

func TestBoxPlot(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Box,
		ColumnX: "value",
		Width:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Terminal, "Median") {
		t.Error("box plot should show median")
	}
}

func TestHeatmap(t *testing.T) {
	ds := makeTestDS()
	c, err := Render(ds, Config{
		Type:    Heatmap,
		ColumnX: "value",
		ColumnY: "count",
		Width:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Terminal) == 0 {
		t.Error("heatmap should produce output")
	}
}

func TestRender_NoData(t *testing.T) {
	_, err := Render(nil, Config{Type: Bar})
	if err == nil {
		t.Error("expected error for nil dataset")
	}
}

func TestRender_UnknownType(t *testing.T) {
	ds := makeTestDS()
	_, err := Render(ds, Config{Type: ChartType("unknown")})
	if err == nil {
		t.Error("expected error for unknown chart type")
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input interface{}
		want  float64
		ok    bool
	}{
		{float64(3.14), 3.14, true},
		{int(42), 42.0, true},
		{int64(99), 99.0, true},
		{"3.5", 3.5, true},
		{"abc", 0, false},
	}
	for _, tt := range tests {
		got, err := toFloat64(tt.input)
		if tt.ok && err != nil {
			t.Errorf("toFloat64(%v) unexpected error: %v", tt.input, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("toFloat64(%v) expected error", tt.input)
		}
		if tt.ok && got != tt.want {
			t.Errorf("toFloat64(%v) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestUniqueStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	result := uniqueStrings(input)
	if len(result) != 3 {
		t.Errorf("expected 3 unique, got %d", len(result))
	}
}

func TestIndexOf(t *testing.T) {
	if idx := indexOf("b", []string{"a", "b", "c"}); idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}
	if idx := indexOf("z", []string{"a", "b"}); idx != -1 {
		t.Errorf("expected -1, got %d", idx)
	}
}
