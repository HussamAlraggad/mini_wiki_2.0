// Package ranking implements relevance scoring of dataset rows against a user-provided
// research topic. It uses the local LLM to score each row's relevance and supports
// iterative comparison and threshold-based discarding.
package ranking

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mini-wiki/internal/dataset"

	_ "modernc.org/sqlite"
)

// ScoreRecord stores a single row's score for persistence.
type ScoreRecord struct {
	RowIndex int     `json:"row_index"`
	Score    float64 `json:"score"`
	Topic    string  `json:"topic"`
}

// RankResult is what /rank produces and /chart consumes.
type RankResult struct {
	Dataset      *dataset.Dataset // original data + "relevance_score" column appended
	Topic        string           // the topic used for scoring
	Scores       []float64        // one score per row (same order as Dataset.Rows)
	MeanScore    float64
	MinScore     float64
	MaxScore     float64
	DiscardCount int // number of rows discarded (if /discard was run)
}

// Ranker performs relevance scoring against a topic.
type Ranker interface {
	// ScoreAll scores every row in the dataset against the topic.
	// It calls the LLM once per row (or in batches) and returns scores.
	ScoreAll(ctx context.Context, data *dataset.Dataset, topic string) (*RankResult, error)

	// Rerank scores against a refined topic, preserving the original scores for comparison.
	Rerank(ctx context.Context, original *RankResult, newTopic string) (*RankResult, error)
}

// LLMClient is the interface for calling the local LLM.
type LLMClient interface {
	// Generate sends a prompt to the LLM and returns the response text.
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// Config controls ranking behavior.
type Config struct {
	Model          string  // LLM model to use
	MaxRows        int     // max rows to score (default 10000)
	TruncateMsg    string  // warning when truncated
}

// DefaultConfig returns sensible defaults for ranking.
func DefaultConfig() Config {
	return Config{
		Model:   "qwen2.5-coder",
		MaxRows: 10000,
	}
}

// NewRanker creates a new Ranker.
func NewRanker(client LLMClient, cfg Config) Ranker {
	return &ranker{client: client, cfg: cfg}
}

type ranker struct {
	client LLMClient
	cfg    Config
}

// scorePrompt generates the LLM prompt for scoring a single row.
func scorePrompt(topic string, row dataset.Row, columns []dataset.Column) string {
	var rowStr strings.Builder
	rowStr.WriteString(fmt.Sprintf("Row #%d:\n", row.Index))
	for _, col := range columns {
		val := fmt.Sprintf("%v", row.Data[col.Name])
		if len(val) > 200 {
			val = val[:197] + "..."
		}
		rowStr.WriteString(fmt.Sprintf("  %s: %s\n", col.Name, val))
	}

	return fmt.Sprintf(`You are scoring dataset rows for relevance to a research topic.

Research topic: "%s"

Rate how relevant this specific row is to the research topic on a scale of 0.0 to 1.0.
- 0.0 = completely irrelevant
- 0.5 = somewhat relevant
- 1.0 = highly relevant

%s
Respond with ONLY a single number between 0.0 and 1.0. Do not include any other text.`, topic, rowStr.String())
}

func (r *ranker) ScoreAll(ctx context.Context, data *dataset.Dataset, topic string) (*RankResult, error) {
	if data == nil || data.RowCount == 0 {
		return nil, fmt.Errorf("no data to rank")
	}

	rows := data.Rows
	if len(rows) > r.cfg.MaxRows {
		rows = rows[:r.cfg.MaxRows]
	}

	scores := make([]float64, len(rows))
	model := r.cfg.Model
	if model == "" {
		model = "qwen2.5-coder"
	}

	for i, row := range rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		prompt := scorePrompt(topic, row, data.Columns)
		resp, err := r.client.Generate(ctx, model, prompt)
		if err != nil {
			// On error, default to 0.0 and continue
			scores[i] = 0.0
			continue
		}

		score := parseScore(resp)
		scores[i] = score
	}

	// Build RankResult with sorted data
	result := buildRankResult(data, topic, scores, rows)

	return result, nil
}

func (r *ranker) Rerank(ctx context.Context, original *RankResult, newTopic string) (*RankResult, error) {
	return r.ScoreAll(ctx, original.Dataset, newTopic)
}

// parseScore extracts a float from the LLM's response.
func parseScore(resp string) float64 {
	resp = strings.TrimSpace(resp)
	// Try to find a float in the response
	var score float64
	if _, err := fmt.Sscanf(resp, "%f", &score); err != nil {
		// Try to find number in string like "0.85"
		for _, part := range strings.Fields(resp) {
			if _, err := fmt.Sscanf(part, "%f", &score); err == nil {
				break
			}
		}
	}

	// Clamp to [0, 1]
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// buildRankResult creates a RankResult sorted by score descending.
func buildRankResult(data *dataset.Dataset, topic string, scores []float64, rows []dataset.Row) *RankResult {
	// Create a copy of the dataset with relevance_score column
	resultDS := &dataset.Dataset{
		Name:       data.Name,
		SourceFile: data.SourceFile,
		Columns:    append([]dataset.Column(nil), data.Columns...),
		ColumnCount: data.ColumnCount + 1,
		IngestedAt: time.Now(),
	}

	// Add relevance_score column
	resultDS.Columns = append(resultDS.Columns, dataset.Column{
		Name: "relevance_score",
		Kind: dataset.ColumnFloat,
	})

	// Sort indices by score descending
	type scoredRow struct {
		index int
		row   dataset.Row
		score float64
	}
	scored := make([]scoredRow, len(rows))
	for i, row := range rows {
		row.Data["relevance_score"] = scores[i]
		scored[i] = scoredRow{index: i, row: row, score: scores[i]}
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Build sorted rows and compute stats
	resultDS.Rows = make([]dataset.Row, len(scored))
	var sum, min, max float64
	min = math.MaxFloat64
	for i, sr := range scored {
		resultDS.Rows[i] = sr.row
		sum += sr.score
		if sr.score < min {
			min = sr.score
		}
		if sr.score > max {
			max = sr.score
		}
	}
	resultDS.RowCount = len(resultDS.Rows)

	n := float64(len(scores))
	mean := 0.0
	if n > 0 {
		mean = sum / n
	}
	if min > max {
		min = 0
	}

	return &RankResult{
		Dataset:   resultDS,
		Topic:     topic,
		Scores:    scores,
		MeanScore: math.Round(mean*100) / 100,
		MinScore:  math.Round(min*100) / 100,
		MaxScore:  math.Round(max*100) / 100,
	}
}

// ResultsToJSON serializes score records to JSON.
func ResultsToJSON(scores []float64, topic string) string {
	records := make([]ScoreRecord, len(scores))
	for i, s := range scores {
		records[i] = ScoreRecord{
			RowIndex: i,
			Score:    s,
			Topic:    topic,
		}
	}
	data, _ := json.Marshal(records)
	return string(data)
}

// FormatRankingTable returns a formatted string of the top-ranked rows.
func FormatRankingTable(result *RankResult, limit int) string {
	if result == nil {
		return "No ranking data."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Ranking for: %s\n", result.Topic))
	b.WriteString(fmt.Sprintf("Rows: %d | Score range: %.2f - %.2f (mean: %.2f)\n\n",
		len(result.Scores), result.MinScore, result.MaxScore, result.MeanScore))

	if limit <= 0 || limit > len(result.Dataset.Rows) {
		limit = len(result.Dataset.Rows)
	}

	// Header
	b.WriteString("Rank | Score | Row |")
	for _, col := range result.Dataset.Columns {
		if col.Name != "relevance_score" {
			b.WriteString(" " + col.Name + " |")
		}
	}
	b.WriteString("\n")

	// Separator
	b.WriteString("-----+-------+-----+")
	for _, col := range result.Dataset.Columns {
		if col.Name != "relevance_score" {
			b.WriteString(strings.Repeat("-", len(col.Name)+1) + "+")
		}
	}
	b.WriteString("\n")

	// Top N rows
	for i := 0; i < limit && i < len(result.Dataset.Rows); i++ {
		row := result.Dataset.Rows[i]
		score := 0.0
		if s, ok := row.Data["relevance_score"].(float64); ok {
			score = s
		}
		b.WriteString(fmt.Sprintf("%4d | %5.2f | %3d |", i+1, score, row.Index))
		for _, col := range result.Dataset.Columns {
			if col.Name == "relevance_score" {
				continue
			}
			val := fmt.Sprintf("%v", row.Data[col.Name])
			if len(val) > 20 {
				val = val[:17] + "..."
			}
			b.WriteString(" " + val + " |")
		}
		b.WriteString("\n")
	}

	if len(result.Dataset.Rows) > limit {
		b.WriteString(fmt.Sprintf("... %d more rows\n", len(result.Dataset.Rows)-limit))
	}

	return b.String()
}

// FormatComparison returns a side-by-side comparison of two rankings.
func FormatComparison(prev, curr *RankResult) string {
	if prev == nil || curr == nil {
		return "Need two rankings to compare."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Previous: %s\n", prev.Topic))
	b.WriteString(fmt.Sprintf("Current:  %s\n\n", curr.Topic))
	b.WriteString(fmt.Sprintf("Previous | Score | Current | Score | Delta\n"))
	b.WriteString("---------+-------+---------+-------+-------\n")

	maxRows := len(prev.Dataset.Rows)
	if len(curr.Dataset.Rows) < maxRows {
		maxRows = len(curr.Dataset.Rows)
	}
	if maxRows > 20 {
		maxRows = 20
	}

	for i := 0; i < maxRows; i++ {
		pRow := prev.Dataset.Rows[i]
		cRow := curr.Dataset.Rows[i]
		pScore, _ := pRow.Data["relevance_score"].(float64)
		cScore, _ := cRow.Data["relevance_score"].(float64)
		delta := cScore - pScore

		pVal := fmt.Sprintf("%v", pRow.Data[pRow.Data["col_0"].(string)])
		cVal := fmt.Sprintf("%v", cRow.Data[cRow.Data["col_0"].(string)])

		if len(pVal) > 15 {
			pVal = pVal[:12] + "..."
		}
		if len(cVal) > 15 {
			cVal = cVal[:12] + "..."
		}

		deltaStr := fmt.Sprintf("%+.2f", delta)
		b.WriteString(fmt.Sprintf("%-8s | %5.2f | %-8s | %5.2f | %s\n",
			pVal, pScore, cVal, cScore, deltaStr))
	}

	return b.String()
}

// LoadDataset reads the currently active ingested dataset from the project KB.
// This is the Phase 3 -> Phase 4 boundary.
func LoadDataset(projectDir string) (*dataset.Dataset, error) {
	if projectDir == "" {
		return nil, fmt.Errorf("no project directory set")
	}

	// Open project KB
	kbPath := filepath.Join(projectDir, ".wiki", "kb.sqlite")
	db, err := sql.Open("sqlite", kbPath)
	if err != nil {
		return nil, fmt.Errorf("open project kb: %w", err)
	}
	defer db.Close()

	// Query active dataset
	var filePath, fileFormat string
	err = db.QueryRow(`SELECT file_path, file_format FROM active_dataset ORDER BY id DESC LIMIT 1`).Scan(&filePath, &fileFormat)
	if err != nil {
		return nil, fmt.Errorf("no dataset ingested. Use /ingest first.")
	}

	// Re-parse the file based on format
	switch fileFormat {
	case "csv":
		return parseCSV(filePath)
	case "jsonl":
		return parseJSONL(filePath)
	default:
		return parseText(filePath)
	}
}

// parseCSV re-parses a CSV file into a *dataset.Dataset.
func parseCSV(path string) (*dataset.Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(bufio.NewReader(f))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}

	ds := &dataset.Dataset{
		Name:       filepath.Base(path),
		SourceFile: path,
		IngestedAt: time.Now(),
	}

	for _, h := range headers {
		ds.Columns = append(ds.Columns, dataset.Column{
			Name: h,
			Kind: dataset.ColumnString,
		})
	}
	ds.ColumnCount = len(ds.Columns)

	// Read rows
	rowIdx := 0
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		row := dataset.Row{
			Index: rowIdx,
			Data:  make(map[string]interface{}),
		}
		for i, val := range record {
			if i < len(headers) {
				row.Data[headers[i]] = val
			}
		}
		ds.Rows = append(ds.Rows, row)
		rowIdx++
	}
	ds.RowCount = len(ds.Rows)

	// Detect column types from first 100 rows
	detectColumnTypes(ds)

	return ds, nil
}

// parseJSONL re-parses a JSONL file into a *dataset.Dataset.
func parseJSONL(path string) (*dataset.Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open jsonl: %w", err)
	}
	defer f.Close()

	ds := &dataset.Dataset{
		Name:       filepath.Base(path),
		SourceFile: path,
		IngestedAt: time.Now(),
	}

	scanner := bufio.NewScanner(f)
	fieldSet := make(map[string]bool)
	var fields []string
	rowIdx := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		// Collect field names from first row
		if len(fieldSet) == 0 {
			for k := range data {
				fieldSet[k] = true
				fields = append(fields, k)
			}
			for _, f := range fields {
				ds.Columns = append(ds.Columns, dataset.Column{
					Name: f,
					Kind: dataset.ColumnString,
				})
			}
			ds.ColumnCount = len(ds.Columns)
		}

		row := dataset.Row{
			Index: rowIdx,
			Data:  make(map[string]interface{}),
		}
		for _, field := range fields {
			if val, ok := data[field]; ok {
				row.Data[field] = val
			}
		}
		ds.Rows = append(ds.Rows, row)
		rowIdx++
	}
	ds.RowCount = len(ds.Rows)
	detectColumnTypes(ds)

	return ds, nil
}

// parseText reads a plain text file as a single-column dataset.
func parseText(path string) (*dataset.Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read text: %w", err)
	}

	ds := &dataset.Dataset{
		Name:       filepath.Base(path),
		SourceFile: path,
		Columns: []dataset.Column{
			{Name: "text", Kind: dataset.ColumnString},
		},
		ColumnCount: 1,
		IngestedAt:  time.Now(),
		Rows: []dataset.Row{
			{Index: 0, Data: map[string]interface{}{"text": string(data)}},
		},
		RowCount: 1,
	}
	return ds, nil
}

// detectColumnTypes samples the first 100 rows to infer column types.
func detectColumnTypes(ds *dataset.Dataset) {
	maxRows := 100
	if len(ds.Rows) < maxRows {
		maxRows = len(ds.Rows)
	}
	for i := range ds.Columns {
		allInt := true
		allFloat := true
		for _, row := range ds.Rows[:maxRows] {
			val, ok := row.Data[ds.Columns[i].Name]
			if !ok {
				continue
			}
			s := fmt.Sprintf("%v", val)
			// Check if it contains a decimal point or exponent (float indicator)
			isFloatFormatted := strings.Contains(s, ".") || strings.Contains(s, "e") || strings.Contains(s, "E")
			// Try integer
			var iv int64
			if _, err := fmt.Sscanf(s, "%d", &iv); err != nil || isFloatFormatted {
				allInt = false
			}
			// Try float
			var fv float64
			if _, err := fmt.Sscanf(s, "%f", &fv); err != nil {
				allFloat = false
			}
		}
		if allInt {
			ds.Columns[i].Kind = dataset.ColumnInteger
		} else if allFloat {
			ds.Columns[i].Kind = dataset.ColumnFloat
		}
	}
}
