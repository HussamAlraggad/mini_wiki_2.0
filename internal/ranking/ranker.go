// Package ranking implements relevance scoring of dataset rows against a user-provided
// research topic. It uses Agentic RAG: sends the schema to a coder LLM, which writes
// a Pandas filter script. The script is executed locally for O(1) LLM cost.
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
	"mini-wiki/internal/rag"

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

// RagClient is the interface the ranker uses to communicate with the RAG worker.
type RagClient interface {
	Rank(path, topic, llmModel string) (*rag.Response, error)
}

// Ranker performs relevance scoring against a topic.
type Ranker interface {
	// ScoreAll scores every row in the dataset against the topic.
	// Uses Agentic RAG: sends schema to coder LLM, executes Pandas script locally.
	ScoreAll(ctx context.Context, data *dataset.Dataset, topic string) (*RankResult, error)

	// Rerank scores against a refined topic, preserving the original scores for comparison.
	Rerank(ctx context.Context, original *RankResult, newTopic string) (*RankResult, error)
}

// Config controls ranking behavior.
type Config struct {
	Model       string // LLM model for code generation (default: qwen2.5-coder:7b)
	CodeGenOnly bool   // if true, skip dataset loading and just return generated code
}

// DefaultConfig returns sensible defaults for ranking.
func DefaultConfig() Config {
	return Config{
		Model: "qwen2.5-coder:7b",
	}
}

// NewRanker creates a new Ranker that uses the RAG worker for agentic ranking.
func NewRanker(ragClient RagClient, cfg Config) Ranker {
	return &ranker{ragClient: ragClient, cfg: cfg}
}

type ranker struct {
	ragClient RagClient
	cfg       Config
}

func (r *ranker) ScoreAll(ctx context.Context, data *dataset.Dataset, topic string) (*RankResult, error) {
	if data == nil || data.RowCount == 0 {
		return nil, fmt.Errorf("no data to rank")
	}

	if r.ragClient == nil {
		return nil, fmt.Errorf("RAG worker not available for agentic ranking")
	}

	model := r.cfg.Model
	if model == "" {
		model = "qwen2.5-coder:7b"
	}

	// Use the Agentic RAG approach: send the dataset path to the Python worker.
	// The worker loads the data, extracts schema, prompts the coder LLM for a
	// Pandas filter script, executes it locally, and returns filtered results.
	rankResp, err := r.ragClient.Rank(data.SourceFile, topic, model)
	if err != nil {
		return nil, fmt.Errorf("agentic rank failed: %w", err)
	}

	if rankResp.Type == "error" {
		errMsg := rankResp.Error
		if errMsg == "" {
			errMsg = rankResp.Message
		}
		return nil, fmt.Errorf("agentic rank error: %s", errMsg)
	}

	// Convert the Python worker's response into a RankResult
	result := buildRankResultFromAgentic(data, topic, rankResp)

	return result, nil
}

func (r *ranker) Rerank(ctx context.Context, original *RankResult, newTopic string) (*RankResult, error) {
	return r.ScoreAll(ctx, original.Dataset, newTopic)
}

// buildRankResultFromAgentic converts the Python worker's response into a RankResult.
// The worker returns filtered data with relevance_score already computed by the
// generated Pandas script.
func buildRankResultFromAgentic(originalDS *dataset.Dataset, topic string, resp *rag.Response) *RankResult {
	// Create a new dataset from the filtered results
	resultDS := &dataset.Dataset{
		Name:        originalDS.Name,
		SourceFile:  originalDS.SourceFile,
		Columns:     append([]dataset.Column(nil), originalDS.Columns...),
		ColumnCount: originalDS.ColumnCount + 1,
		IngestedAt:  time.Now(),
	}

	// Add relevance_score column (may already be in data, but ensure it's tracked)
	hasScoreCol := false
	for _, c := range resultDS.Columns {
		if c.Name == "relevance_score" {
			hasScoreCol = true
			break
		}
	}
	if !hasScoreCol {
		resultDS.Columns = append(resultDS.Columns, dataset.Column{
			Name: "relevance_score",
			Kind: dataset.ColumnFloat,
		})
		resultDS.ColumnCount++
	}

	// Build rows from the returned data
	scores := make([]float64, len(resp.Data))
	for i, item := range resp.Data {
		row := dataset.Row{
			Index: i,
			Data:  make(map[string]interface{}),
		}
		for k, v := range item {
			row.Data[k] = v
		}
		// Extract relevance_score
		if s, ok := item["relevance_score"].(float64); ok {
			scores[i] = s
		} else if s, ok := item["relevance_score"].(int); ok {
			scores[i] = float64(s)
		} else if s, ok := item["relevance_score"].(int64); ok {
			scores[i] = float64(s)
		} else if s, ok := item["relevance_score"].(int32); ok {
			scores[i] = float64(s)
		}
		resultDS.Rows = append(resultDS.Rows, row)
	}
	resultDS.RowCount = len(resultDS.Rows)

	// Sort by relevance_score descending
	sort.Slice(resultDS.Rows, func(i, j int) bool {
		si := 0.0
		sj := 0.0
		if s, ok := resultDS.Rows[i].Data["relevance_score"].(float64); ok {
			si = s
		}
		if s, ok := resultDS.Rows[j].Data["relevance_score"].(float64); ok {
			sj = s
		}
		return si > sj
	})

	// Re-sort scores to match sorted rows
	sort.Slice(scores, func(i, j int) bool {
		return scores[i] > scores[j]
	})

	// Compute stats
	var sum, min, max float64
	min = math.MaxFloat64
	for _, s := range scores {
		sum += s
		if s < min {
			min = s
		}
		if s > max {
			max = s
		}
	}
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

	// Use a large buffer (20MB) to handle JSONL lines up to 12MB+.
	const maxLineSize = 20 * 1024 * 1024
	scanner := bufio.NewScanner(f)
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

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
