// Package chart provides ASCII-terminal and file-based data visualization.
// Charts are rendered as clean terminal graphics using block characters,
// auto-scaling to terminal width.
package chart

import (
	"fmt"
	"sort"
	"strings"

	"mini-wiki/internal/dataset"
)

// ChartType identifies what kind of chart to render.
type ChartType string

const (
	Bar       ChartType = "bar"
	Trend     ChartType = "trend"
	Pie       ChartType = "pie"
	Scatter   ChartType = "scatter"
	Histogram ChartType = "histogram"
	Box       ChartType = "box"
	Heatmap   ChartType = "heatmap"
)

// Config controls chart rendering.
type Config struct {
	Type       ChartType
	ColumnX    string // x-axis column (or primary column)
	ColumnY    string // y-axis column (for scatter)
	Buckets    int    // for histogram
	Width      int    // terminal width (0 = auto)
	Height     int    // terminal height (0 = auto)
	Title      string
	ExportPath string // if set, export to file
}

// Chart is the main chart object.
type Chart struct {
	Config   Config
	Dataset  *dataset.Dataset
	Title    string
	Terminal string // ASCII terminal rendering
}

// Render generates the ASCII terminal rendering of the chart.
func Render(ds *dataset.Dataset, cfg Config) (*Chart, error) {
	if ds == nil || ds.RowCount == 0 {
		return nil, fmt.Errorf("no data to chart")
	}
	if cfg.Width <= 0 {
		cfg.Width = 60
	}
	if cfg.Height <= 0 {
		cfg.Height = 16
	}

	c := &Chart{
		Config:  cfg,
		Dataset: ds,
	}

	switch cfg.Type {
	case Bar:
		return c.renderBar()
	case Trend:
		return c.renderTrend()
	case Pie:
		return c.renderPie()
	case Scatter:
		return c.renderScatter()
	case Histogram:
		return c.renderHistogram()
	case Box:
		return c.renderBox()
	case Heatmap:
		return c.renderHeatmap()
	default:
		return nil, fmt.Errorf("unknown chart type: %s", cfg.Type)
	}
}

// --- Bar Chart ---

func (c *Chart) renderBar() (*Chart, error) {
	values, labels, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no numeric data in column %q", c.Config.ColumnX)
	}

	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	barWidth := c.Config.Width - 10 // leave room for labels
	if barWidth < 10 {
		barWidth = 10
	}

	var b strings.Builder
	c.writeTitle(&b)

	limit := 20
	if len(labels) > limit {
		labels = labels[:limit]
		values = values[:limit]
	}

	for i := 0; i < len(values) && i < limit; i++ {
		barLen := int((values[i] / maxVal) * float64(barWidth))
		if barLen < 1 && values[i] > 0 {
			barLen = 1
		}
		label := labels[i]
		if len(label) > 8 {
			label = label[:8]
		}
		bar := strings.Repeat("█", barLen)
		b.WriteString(fmt.Sprintf("%8s │%s %.1f\n", label, bar, values[i]))
	}
	b.WriteString(strings.Repeat("─", barWidth+10) + "\n")

	c.Terminal = b.String()
	return c, nil
}

// --- Trend / Line Chart ---

func (c *Chart) renderTrend() (*Chart, error) {
	values, labels, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no numeric data in column %q", c.Config.ColumnX)
	}

	width := c.Config.Width - 6
	height := c.Config.Height - 3

	if width < 10 {
		width = 10
	}
	if height < 5 {
		height = 5
	}

	maxVal, minVal := values[0], values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
		if v < minVal {
			minVal = v
		}
	}
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	range_ := maxVal - minVal

	// Build grid
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Plot points
	if len(values) > 1 {
		step := float64(width-1) / float64(len(values)-1)
		for i := 0; i < len(values) && i < width; i++ {
			x := int(float64(i) * step)
			if x >= width {
				x = width - 1
			}
			y := int(((values[i] - minVal) / range_) * float64(height-1))
			if y >= height {
				y = height - 1
			}
			if y < 0 {
				y = 0
			}
			grid[height-1-y][x] = '*'
		}

		// Connect points with lines
		for i := 0; i < len(values)-1 && i < width-1; i++ {
			x1 := int(float64(i) * step)
			x2 := int(float64(i+1) * step)
			if x1 >= width || x2 >= width {
				break
			}
			y1 := int(((values[i] - minVal) / range_) * float64(height-1))
			y2 := int(((values[i+1] - minVal) / range_) * float64(height-1))
			if y1 >= height {
				y1 = height - 1
			}
			if y2 >= height {
				y2 = height - 1
			}
			if y1 < 0 {
				y1 = 0
			}
			if y2 < 0 {
				y2 = 0
			}
			// Draw line between (x1, y1) and (x2, y2)
			drawLine(grid, x1, height-1-y1, x2, height-1-y2, width, height)
		}
	}

	var b strings.Builder
	c.writeTitle(&b)

	// Y-axis labels
	yLabels := 4
	for i := 0; i < yLabels; i++ {
		val := maxVal - (float64(i)/float64(yLabels-1))*range_
		row := int(float64(i) * float64(height-1) / float64(yLabels-1))
		if row >= height {
			row = height - 1
		}
		b.WriteString(fmt.Sprintf("%6.1f │", val))
		b.WriteString(string(grid[row]))
		b.WriteString("\n")
	}
	b.WriteString("       └" + strings.Repeat("─", width) + "\n")

	if len(labels) > 0 {
		b.WriteString(fmt.Sprintf("       %s ... %s\n", labels[0], labels[len(labels)-1]))
	}

	c.Terminal = b.String()
	return c, nil
}

// --- Pie Chart ---

func (c *Chart) renderPie() (*Chart, error) {
	values, labels, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}

	total := 0.0
	for _, v := range values {
		total += v
	}
	if total == 0 {
		total = 1
	}

	// Group small slices into "other"
	type slice struct {
		label string
		pct   float64
	}
	var slices []slice
	otherPct := 0.0
	limit := 8
	for i, v := range values {
		pct := (v / total) * 100
		if i < limit-1 {
			slices = append(slices, slice{label: labels[i], pct: pct})
		} else {
			otherPct += pct
		}
	}
	if otherPct > 0 {
		slices = append(slices, slice{label: "other", pct: otherPct})
	}

	var b strings.Builder
	c.writeTitle(&b)
	b.WriteString(fmt.Sprintf("  Total: %.0f\n\n", total))

	// Simple bar-style pie
	for _, s := range slices {
		barLen := int(s.pct / 2)
		if barLen < 1 && s.pct > 0 {
			barLen = 1
		}
		bar := strings.Repeat("▓", barLen)
		label := s.label
		if len(label) > 12 {
			label = label[:12]
		}
		b.WriteString(fmt.Sprintf("  %12s │%s %5.1f%%\n", label, bar, s.pct))
	}

	c.Terminal = b.String()
	return c, nil
}

// --- Scatter Plot ---

func (c *Chart) renderScatter() (*Chart, error) {
	xVals, _, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}
	yVals, _, err := c.extractValues(c.Config.ColumnY)
	if err != nil {
		return nil, err
	}

	n := len(xVals)
	if len(yVals) < n {
		n = len(yVals)
	}
	if n == 0 {
		return nil, fmt.Errorf("no data")
	}

	width := c.Config.Width - 8
	height := c.Config.Height - 3
	if width < 10 {
		width = 10
	}
	if height < 5 {
		height = 5
	}

	maxX, minX := xVals[0], xVals[0]
	maxY, minY := yVals[0], yVals[0]
	for i := 0; i < n; i++ {
		if xVals[i] > maxX {
			maxX = xVals[i]
		}
		if xVals[i] < minX {
			minX = xVals[i]
		}
		if yVals[i] > maxY {
			maxY = yVals[i]
		}
		if yVals[i] < minY {
			minY = yVals[i]
		}
	}
	if maxX == minX {
		maxX = minX + 1
	}
	if maxY == minY {
		maxY = minY + 1
	}

	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	for i := 0; i < n; i++ {
		x := int(((xVals[i] - minX) / (maxX - minX)) * float64(width-1))
		y := int(((yVals[i] - minY) / (maxY - minY)) * float64(height-1))
		if x >= width {
			x = width - 1
		}
		if y >= height {
			y = height - 1
		}
		grid[height-1-y][x] = '+'
	}

	var b strings.Builder
	c.writeTitle(&b)
	for _, row := range grid {
		b.WriteString("       │")
		b.WriteString(string(row))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("       └%s\n", strings.Repeat("─", width)))
	b.WriteString(fmt.Sprintf("  X: %s (%.1f-%.1f)  Y: %s (%.1f-%.1f)\n",
		c.Config.ColumnX, minX, maxX, c.Config.ColumnY, minY, maxY))

	c.Terminal = b.String()
	return c, nil
}

// --- Histogram ---

func (c *Chart) renderHistogram() (*Chart, error) {
	values, _, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}

	buckets := c.Config.Buckets
	if buckets <= 0 {
		buckets = 10
	}
	if buckets > 30 {
		buckets = 30
	}

	maxVal, minVal := values[0], values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
		if v < minVal {
			minVal = v
		}
	}
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	range_ := maxVal - minVal
	bucketSize := range_ / float64(buckets)
	counts := make([]int, buckets)

	for _, v := range values {
		idx := int((v - minVal) / bucketSize)
		if idx >= buckets {
			idx = buckets - 1
		}
		if idx < 0 {
			idx = 0
		}
		counts[idx]++
	}

	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	barWidth := c.Config.Width - 14
	if barWidth < 10 {
		barWidth = 10
	}

	var b strings.Builder
	c.writeTitle(&b)
	_ = range_

	for i := 0; i < buckets; i++ {
		low := minVal + float64(i)*bucketSize
		high := low + bucketSize
		barLen := int(float64(counts[i]) / float64(maxCount) * float64(barWidth))
		if barLen < 1 && counts[i] > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)
		b.WriteString(fmt.Sprintf("%6.1f-%5.1f │%s %d\n", low, high, bar, counts[i]))
	}
	b.WriteString("       └" + strings.Repeat("─", barWidth+3) + "\n")

	c.Terminal = b.String()
	return c, nil
}

// --- Box Plot ---

func (c *Chart) renderBox() (*Chart, error) {
	values, labels, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	q1 := sorted[n/4]
	median := sorted[n/2]
	q3 := sorted[3*n/4]
	minVal := sorted[0]
	maxVal := sorted[n-1]
	iqr := q3 - q1

	width := c.Config.Width - 14
	if width < 10 {
		width = 10
	}

	var b strings.Builder
	c.writeTitle(&b)
	b.WriteString(fmt.Sprintf("  Column: %s\n", c.Config.ColumnX))
	b.WriteString(fmt.Sprintf("  Count: %d\n", n))
	b.WriteString(fmt.Sprintf("  Min:   %.2f\n", minVal))
	b.WriteString(fmt.Sprintf("  Q1:    %.2f\n", q1))
	b.WriteString(fmt.Sprintf("  Median: %.2f\n", median))
	b.WriteString(fmt.Sprintf("  Q3:    %.2f\n", q3))
	b.WriteString(fmt.Sprintf("  Max:   %.2f\n", maxVal))
	b.WriteString(fmt.Sprintf("  IQR:   %.2f\n", iqr))
	b.WriteString("\n")

	// Simple visual: min-q1-median-q3-max as a horizontal range
	range_ := maxVal - minVal
	if range_ == 0 {
		range_ = 1
	}
	draw := func(val float64) int {
		return int((val - minVal) / range_ * float64(width))
	}

	minP := draw(minVal)
	q1P := draw(q1)
	medP := draw(median)
	q3P := draw(q3)
	maxP := draw(maxVal)

	line := make([]rune, width+1)
	for i := range line {
		line[i] = '─'
	}
	line[minP] = '├'
	line[q1P] = '┼'
	line[medP] = '◆'
	line[q3P] = '┤'
	line[maxP] = '├'

	// Fill IQR with █
	for i := q1P; i <= q3P && i < len(line); i++ {
		if line[i] == '─' {
			line[i] = '█'
		}
	}

	b.WriteString("  " + string(line) + "\n")
	_ = labels

	c.Terminal = b.String()
	return c, nil
}

// --- Heatmap ---

func (c *Chart) renderHeatmap() (*Chart, error) {
	xVals, xLabels, err := c.extractValues(c.Config.ColumnX)
	if err != nil {
		return nil, err
	}
	yVals, yLabels, err := c.extractValues(c.Config.ColumnY)
	if err != nil {
		return nil, err
	}

	n := len(xVals)
	if len(yVals) < n {
		n = len(yVals)
	}
	if n == 0 {
		return nil, fmt.Errorf("no data")
	}

	// Build unique categories
	xCats := uniqueStrings(xLabels)
	yCats := uniqueStrings(yLabels)

	maxX := len(xCats)
	maxY := len(yCats)
	if maxX > 10 {
		maxX = 10
	}
	if maxY > 10 {
		maxY = 10
	}

	// Build matrix
	matrix := make([][]float64, maxY)
	for i := range matrix {
		matrix[i] = make([]float64, maxX)
	}

	for i := 0; i < n && i < 1000; i++ {
		xi := indexOf(xLabels[i], xCats)
		yi := indexOf(yLabels[i], yCats)
		if xi >= 0 && xi < maxX && yi >= 0 && yi < maxY {
			matrix[yi][xi] += xVals[i]
		}
	}

	maxCell := 0.0
	for _, row := range matrix {
		for _, v := range row {
			if v > maxCell {
				maxCell = v
			}
		}
	}
	if maxCell == 0 {
		maxCell = 1
	}

	var b strings.Builder
	c.writeTitle(&b)

	// Header row
	b.WriteString("         ")
	for _, cat := range xCats[:maxX] {
		c := cat
		if len(c) > 5 {
			c = c[:5]
		}
		b.WriteString(fmt.Sprintf("%6s", c))
	}
	b.WriteString("\n")

	// Rows
	for yi := 0; yi < maxY; yi++ {
		label := yCats[yi]
		if len(label) > 7 {
			label = label[:7]
		}
		b.WriteString(fmt.Sprintf("%8s ", label))
		for xi := 0; xi < maxX; xi++ {
			val := matrix[yi][xi]
			pct := int((val / maxCell) * 9)
			if pct < 0 {
				pct = 0
			}
			if pct > 9 {
				pct = 9
			}
			chars := []string{" ", "░", "▒", "▓", "█", "█", "█", "█", "█", "█"}
			b.WriteString(fmt.Sprintf("  %s  ", chars[pct]))
		}
		b.WriteString("\n")
	}

	c.Terminal = b.String()
	return c, nil
}

// --- Helpers ---

func (c *Chart) writeTitle(b *strings.Builder) {
	if c.Title != "" {
		b.WriteString(fmt.Sprintf("  %s\n\n", c.Title))
	}
}

// extractValues gets numeric values and string labels from a dataset column.
func (c *Chart) extractValues(column string) ([]float64, []string, error) {
	if column == "" {
		return nil, nil, fmt.Errorf("no column specified")
	}
	var values []float64
	var labels []string
	for _, row := range c.Dataset.Rows {
		val, ok := row.Data[column]
		if !ok {
			continue
		}
		f, err := toFloat64(val)
		if err != nil {
			continue
		}
		values = append(values, f)
		// Use first other column as label
		label := ""
		for _, col := range c.Dataset.Columns {
			if col.Name != column {
				if v, ok := row.Data[col.Name]; ok {
					label = fmt.Sprintf("%v", v)
					break
				}
			}
		}
		labels = append(labels, label)
	}
	return values, labels, nil
}

func toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		var f float64
		_, err := fmt.Sscanf(val, "%f", &f)
		return f, err
	default:
		return 0, fmt.Errorf("cannot convert")
	}
}

func drawLine(grid [][]rune, x1, y1, x2, y2, w, h int) {
	dx := x2 - x1
	dy := y2 - y1
	steps := abs(dx)
	if abs(dy) > steps {
		steps = abs(dy)
	}
	if steps == 0 {
		if x1 >= 0 && x1 < w && y1 >= 0 && y1 < h {
			grid[y1][x1] = '*'
		}
		return
	}
	for i := 0; i <= steps; i++ {
		x := x1 + (dx*i)/steps
		y := y1 + (dy*i)/steps
		if x >= 0 && x < w && y >= 0 && y < h {
			grid[y][x] = '*'
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, str := range s {
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}
	return result
}

func indexOf(s string, list []string) int {
	for i, item := range list {
		if item == s {
			return i
		}
	}
	return -1
}
