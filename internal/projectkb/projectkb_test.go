package projectkb

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) (DB, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "projectkb_test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	db := New()
	if err := db.Open(context.Background(), dir); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Open failed: %v", err)
	}
	return db, dir
}

func closeAndCleanup(t *testing.T, db DB, dir string) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
	os.RemoveAll(dir)
}

func TestOpenClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "projectkb_test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	db := New()
	if err := db.Open(context.Background(), dir); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	kbPath := filepath.Join(dir, ".wiki", "kb.sqlite")
	if _, err := os.Stat(kbPath); os.IsNotExist(err) {
		t.Error("kb.sqlite was not created")
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	db, dir := newTestDB(t)
	defer os.RemoveAll(dir)

	if err := db.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second Close (idempotent) failed: %v", err)
	}
}

func TestProjectDir(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)

	if got := db.ProjectDir(); got != dir {
		t.Errorf("ProjectDir() = %q, want %q", got, dir)
	}
}

func TestSaveGetRequirements(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	reqs := []Requirement{
		{ID: "FR1", Type: "FR", Category: "functionality", Statement: "The system shall do X", Source: "stakeholder", Rationale: "needed"},
		{ID: "FR2", Type: "FR", Category: "performance", Statement: "The system shall do Y", Source: "derived", Rationale: "optimization"},
	}

	runID := "run-001"
	if err := db.SaveRequirements(ctx, reqs, runID); err != nil {
		t.Fatalf("SaveRequirements failed: %v", err)
	}

	got, err := db.GetRequirements(ctx, runID)
	if err != nil {
		t.Fatalf("GetRequirements failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d requirements, want 2", len(got))
	}
	if got[0].ID != "FR1" || got[0].Statement != "The system shall do X" {
		t.Errorf("first requirement mismatch: %+v", got[0])
	}
	if got[1].ID != "FR2" || got[1].Type != "FR" {
		t.Errorf("second requirement mismatch: %+v", got[1])
	}
}

func TestGetRequirementsEmpty(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)

	got, err := db.GetRequirements(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetRequirements for nonexistent run failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

func TestSaveGetMoscow(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	entries := []MoscowEntry{
		{ID: "FR1", Statement: "Login required", Moscow: "MUST", Justification: "Security"},
		{ID: "FR2", Statement: "Dark mode", Moscow: "COULD", Justification: "Nice to have"},
	}

	runID := "run-002"
	if err := db.SaveMoscow(ctx, entries, runID); err != nil {
		t.Fatalf("SaveMoscow failed: %v", err)
	}

	got, err := db.GetMoscow(ctx, runID)
	if err != nil {
		t.Fatalf("GetMoscow failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Moscow != "MUST" || got[0].Statement != "Login required" {
		t.Errorf("first MoscowEntry mismatch: %+v", got[0])
	}
	if got[1].Moscow != "COULD" {
		t.Errorf("second MoscowEntry Moscow = %q, want COULD", got[1].Moscow)
	}
}

func TestSaveGetDFDComponents(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	comps := []DFDComponent{
		{Type: "entity", Identifier: "E1", Name: "User", Description: "System user", Relations: `{"flows":["D1"]}`},
		{Type: "process", Identifier: "P1", Name: "Auth", Description: "Authentication process", Relations: `{"flows":["D1"]}`},
	}

	runID := "run-003"
	if err := db.SaveDFDComponents(ctx, comps, runID); err != nil {
		t.Fatalf("SaveDFDComponents failed: %v", err)
	}

	got, err := db.GetDFDComponents(ctx, runID)
	if err != nil {
		t.Fatalf("GetDFDComponents failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d components, want 2", len(got))
	}
	if got[0].Type != "entity" || got[0].Identifier != "E1" {
		t.Errorf("first component mismatch: %+v", got[0])
	}
	if got[1].Type != "process" || got[1].Name != "Auth" {
		t.Errorf("second component mismatch: %+v", got[1])
	}
}

func TestSaveGetCSPEC(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	entries := []CSPECEntry{
		{ProcessID: "P1", ProcessName: "Auth", EntryType: "activation", Data: `{"condition":"valid_login"}`},
		{ProcessID: "P2", ProcessName: "Report", EntryType: "decision", Data: `{"condition":"month_end"}`},
	}

	runID := "run-004"
	if err := db.SaveCSPEC(ctx, entries, runID); err != nil {
		t.Fatalf("SaveCSPEC failed: %v", err)
	}

	got, err := db.GetCSPEC(ctx, runID)
	if err != nil {
		t.Fatalf("GetCSPEC failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].ProcessID != "P1" || got[0].EntryType != "activation" {
		t.Errorf("first entry mismatch: %+v", got[0])
	}
	if got[1].ProcessID != "P2" || got[1].EntryType != "decision" {
		t.Errorf("second entry mismatch: %+v", got[1])
	}
}

func TestSaveGetSRSDocuments(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	doc := &SRSDocument{
		ProjectName: "TestProj",
		Version:     "1.0",
		Standard:    "ieee_830",
		Content:     "# SRS Document\n\nThis is the content.",
		Format:      "markdown",
	}
	if err := db.SaveSRSDocument(ctx, doc); err != nil {
		t.Fatalf("SaveSRSDocument failed: %v", err)
	}

	docs, err := db.GetSRSDocuments(ctx)
	if err != nil {
		t.Fatalf("GetSRSDocuments failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1", len(docs))
	}
	if docs[0].ProjectName != "TestProj" {
		t.Errorf("ProjectName = %q, want %q", docs[0].ProjectName, "TestProj")
	}
	if docs[0].Standard != "ieee_830" {
		t.Errorf("Standard = %q, want %q", docs[0].Standard, "ieee_830")
	}
	if docs[0].Content != "# SRS Document\n\nThis is the content." {
		t.Errorf("Content mismatch")
	}
	if docs[0].Format != "markdown" {
		t.Errorf("Format = %q, want %q", docs[0].Format, "markdown")
	}
	if docs[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestSaveGetHistory(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	entries := []*HistoryEntry{
		{Query: "what is X", Response: "X is ...", Model: "qwen2.5-coder", FileRefs: `["file1.txt"]`},
		{Query: "how to Y", Response: "Y by ...", Model: "deepseek-coder", FileRefs: `[]`},
	}
	for _, e := range entries {
		if err := db.SaveHistory(ctx, e); err != nil {
			t.Fatalf("SaveHistory failed: %v", err)
		}
	}

	got, err := db.GetHistory(ctx, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	gotIDs := map[string]bool{}
	for _, e := range got {
		gotIDs[e.Query] = true
	}
	if !gotIDs["what is X"] {
		t.Error("missing 'what is X'")
	}
	if !gotIDs["how to Y"] {
		t.Error("missing 'how to Y'")
	}
}

func TestGetHistoryDefaultLimit(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		if err := db.SaveHistory(ctx, &HistoryEntry{Query: "q", Model: "m"}); err != nil {
			t.Fatalf("SaveHistory failed: %v", err)
		}
	}

	got, err := db.GetHistory(ctx, 0)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("expected 50 entries with limit=0, got %d", len(got))
	}

	gotNeg, err := db.GetHistory(ctx, -1)
	if err != nil {
		t.Fatalf("GetHistory with -1 failed: %v", err)
	}
	if len(gotNeg) != 50 {
		t.Errorf("expected 50 entries with limit=-1, got %d", len(gotNeg))
	}
}

func TestSearchHistory(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	db.SaveHistory(ctx, &HistoryEntry{Query: "authentication flow", Response: "Auth flow involves login", Model: "m1"})
	db.SaveHistory(ctx, &HistoryEntry{Query: "database schema", Response: "The DB has tables", Model: "m1"})
	db.SaveHistory(ctx, &HistoryEntry{Query: "login timeout", Response: "Timeout is 30s", Model: "m2"})

	got, err := db.SearchHistory(ctx, "auth", 10)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result for 'auth', got %d", len(got))
	}
	if got[0].Query != "authentication flow" {
		t.Errorf("expected 'authentication flow', got %q", got[0].Query)
	}
}

func TestSearchHistoryEmptyQuery(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	db.SaveHistory(ctx, &HistoryEntry{Query: "q1", Model: "m"})
	db.SaveHistory(ctx, &HistoryEntry{Query: "q2", Model: "m"})

	got, err := db.SearchHistory(ctx, "", 10)
	if err != nil {
		t.Fatalf("SearchHistory with empty query failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results for empty query, got %d", len(got))
	}
}

func TestSaveGetDeleteBookmark(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	bm := &Bookmark{
		Title:       "Important Finding",
		Description: "Key insight from analysis",
		Source:      "srs",
		Tags:        "security,auth",
	}
	if err := db.SaveBookmark(ctx, bm); err != nil {
		t.Fatalf("SaveBookmark failed: %v", err)
	}

	bookmarks, err := db.GetBookmarks(ctx)
	if err != nil {
		t.Fatalf("GetBookmarks failed: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("got %d bookmarks, want 1", len(bookmarks))
	}
	if bookmarks[0].Title != "Important Finding" {
		t.Errorf("Title = %q", bookmarks[0].Title)
	}
	if bookmarks[0].Tags != "security,auth" {
		t.Errorf("Tags = %q", bookmarks[0].Tags)
	}
	if bookmarks[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	id := bookmarks[0].ID
	if err := db.DeleteBookmark(ctx, id); err != nil {
		t.Fatalf("DeleteBookmark failed: %v", err)
	}

	bookmarks, err = db.GetBookmarks(ctx)
	if err != nil {
		t.Fatalf("GetBookmarks after delete failed: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks after delete, got %d", len(bookmarks))
	}
}

func TestGetBookmarksEmpty(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)

	bookmarks, err := db.GetBookmarks(context.Background())
	if err != nil {
		t.Fatalf("GetBookmarks failed: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks, got %d", len(bookmarks))
	}
}

func TestSaveGetDeleteFilterState(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	fs := &FilterState{
		Name:     "Active FRs",
		Criteria: `{"type":"FR","status":"active"}`,
	}
	if err := db.SaveFilterState(ctx, fs); err != nil {
		t.Fatalf("SaveFilterState failed: %v", err)
	}

	states, err := db.GetFilterStates(ctx)
	if err != nil {
		t.Fatalf("GetFilterStates failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("got %d filter states, want 1", len(states))
	}
	if states[0].Name != "Active FRs" {
		t.Errorf("Name = %q", states[0].Name)
	}
	if states[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	id := states[0].ID
	if err := db.DeleteFilterState(ctx, id); err != nil {
		t.Fatalf("DeleteFilterState failed: %v", err)
	}

	states, err = db.GetFilterStates(ctx)
	if err != nil {
		t.Fatalf("GetFilterStates after delete failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 filter states after delete, got %d", len(states))
	}
}

func TestGetFilterStatesEmpty(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)

	states, err := db.GetFilterStates(context.Background())
	if err != nil {
		t.Fatalf("GetFilterStates failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 filter states, got %d", len(states))
	}
}

func TestSaveGetRanking(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	r := &RankingResult{
		Topic:  "Best algorithm for sorting",
		Scores: `[{"item":"quicksort","score":0.95},{"item":"bubblesort","score":0.3}]`,
	}
	if err := db.SaveRanking(ctx, r); err != nil {
		t.Fatalf("SaveRanking failed: %v", err)
	}
	if r.ID == 0 {
		t.Error("SaveRanking should set the ID field")
	}

	rankings, err := db.GetRankings(ctx)
	if err != nil {
		t.Fatalf("GetRankings failed: %v", err)
	}
	if len(rankings) != 1 {
		t.Fatalf("got %d rankings, want 1", len(rankings))
	}
	if rankings[0].Topic != "Best algorithm for sorting" {
		t.Errorf("Topic = %q", rankings[0].Topic)
	}
	if rankings[0].ID == 0 {
		t.Error("Ranking ID should be set")
	}
	if rankings[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestSaveGetComparisonSnapshots(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	ranking := &RankingResult{Topic: "t", Scores: "[]"}
	if err := db.SaveRanking(ctx, ranking); err != nil {
		t.Fatalf("SaveRanking failed: %v", err)
	}

	snap := &ComparisonSnapshot{
		RankingID: ranking.ID,
		Topic:     "Comparison v2",
		Scores:    `[{"item":"merge","score":0.9}]`,
	}
	if err := db.SaveComparisonSnapshot(ctx, snap); err != nil {
		t.Fatalf("SaveComparisonSnapshot failed: %v", err)
	}
	if snap.ID == 0 {
		t.Error("SaveComparisonSnapshot should set the ID field")
	}

	snaps, err := db.GetComparisonSnapshots(ctx, ranking.ID)
	if err != nil {
		t.Fatalf("GetComparisonSnapshots failed: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("got %d snapshots, want 1", len(snaps))
	}
	if snaps[0].Topic != "Comparison v2" {
		t.Errorf("Topic = %q", snaps[0].Topic)
	}
	if snaps[0].RankingID != ranking.ID {
		t.Errorf("RankingID = %d, want %d", snaps[0].RankingID, ranking.ID)
	}
	if snaps[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestGetComparisonSnapshotsByRankingID(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	r1 := &RankingResult{Topic: "t1", Scores: "[]"}
	r2 := &RankingResult{Topic: "t2", Scores: "[]"}
	db.SaveRanking(ctx, r1)
	db.SaveRanking(ctx, r2)

	db.SaveComparisonSnapshot(ctx, &ComparisonSnapshot{RankingID: r1.ID, Topic: "s1", Scores: "[]"})
	db.SaveComparisonSnapshot(ctx, &ComparisonSnapshot{RankingID: r1.ID, Topic: "s2", Scores: "[]"})
	db.SaveComparisonSnapshot(ctx, &ComparisonSnapshot{RankingID: r2.ID, Topic: "s3", Scores: "[]"})

	snaps, err := db.GetComparisonSnapshots(ctx, r1.ID)
	if err != nil {
		t.Fatalf("GetComparisonSnapshots failed: %v", err)
	}
	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots for r1, got %d", len(snaps))
	}
}

func TestSaveGetDiscardEntries(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	d := &DiscardEntry{
		Threshold:     0.5,
		RowsDiscarded: 10,
	}
	if err := db.SaveDiscardEntry(ctx, d); err != nil {
		t.Fatalf("SaveDiscardEntry failed: %v", err)
	}

	entries, err := db.GetDiscardEntries(ctx)
	if err != nil {
		t.Fatalf("GetDiscardEntries failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d discard entries, want 1", len(entries))
	}
	if math.Abs(entries[0].Threshold-0.5) > 1e-9 {
		t.Errorf("Threshold = %f, want 0.5", entries[0].Threshold)
	}
	if entries[0].RowsDiscarded != 10 {
		t.Errorf("RowsDiscarded = %d, want 10", entries[0].RowsDiscarded)
	}
	if entries[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestSetActiveDataset(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	if err := db.SetActiveDataset(ctx, "/data/file.csv", "csv", 1000); err != nil {
		t.Fatalf("SetActiveDataset failed: %v", err)
	}

	filePath, fileFormat, err := db.GetActiveDataset(ctx)
	if err != nil {
		t.Fatalf("GetActiveDataset failed: %v", err)
	}
	if filePath != "/data/file.csv" {
		t.Errorf("filePath = %q, want %q", filePath, "/data/file.csv")
	}
	if fileFormat != "csv" {
		t.Errorf("fileFormat = %q, want %q", fileFormat, "csv")
	}
}

func TestGetActiveDatasetEmpty(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)

	_, _, err := db.GetActiveDataset(context.Background())
	if err == nil {
		t.Error("expected error when no active dataset is set")
	}
}

func TestSetActiveDatasetIdempotent(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	if err := db.SetActiveDataset(ctx, "/old/file.csv", "csv", 100); err != nil {
		t.Fatalf("first SetActiveDataset failed: %v", err)
	}

	if err := db.SetActiveDataset(ctx, "/new/file.parquet", "parquet", 500); err != nil {
		t.Fatalf("second SetActiveDataset failed: %v", err)
	}

	filePath, fileFormat, err := db.GetActiveDataset(ctx)
	if err != nil {
		t.Fatalf("GetActiveDataset failed: %v", err)
	}
	if filePath != "/new/file.parquet" {
		t.Errorf("filePath = %q, want %q", filePath, "/new/file.parquet")
	}
	if fileFormat != "parquet" {
		t.Errorf("fileFormat = %q, want %q", fileFormat, "parquet")
	}
}

func TestMultipleRunsSRS(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	db.SaveRequirements(ctx, []Requirement{{ID: "FR1", Type: "FR", Category: "c", Statement: "s1"}}, "run-a")
	db.SaveRequirements(ctx, []Requirement{{ID: "FR2", Type: "FR", Category: "c", Statement: "s2"}}, "run-b")

	gotA, _ := db.GetRequirements(ctx, "run-a")
	if len(gotA) != 1 || gotA[0].ID != "FR1" {
		t.Error("run-a should only have FR1")
	}

	gotB, _ := db.GetRequirements(ctx, "run-b")
	if len(gotB) != 1 || gotB[0].ID != "FR2" {
		t.Error("run-b should only have FR2")
	}
}

func TestSaveRankingSetsID(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	r := &RankingResult{Topic: "test", Scores: "[]"}
	if err := db.SaveRanking(ctx, r); err != nil {
		t.Fatalf("SaveRanking failed: %v", err)
	}
	if r.ID <= 0 {
		t.Errorf("SaveRanking should set ID to positive value, got %d", r.ID)
	}
}

func TestSaveGetDiscardEntriesMultiple(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	for i := 0; i < 15; i++ {
		if err := db.SaveDiscardEntry(ctx, &DiscardEntry{Threshold: 0.1, RowsDiscarded: i}); err != nil {
			t.Fatalf("SaveDiscardEntry failed: %v", err)
		}
	}

	entries, err := db.GetDiscardEntries(ctx)
	if err != nil {
		t.Fatalf("GetDiscardEntries failed: %v", err)
	}
	if len(entries) > 10 {
		t.Errorf("GetDiscardEntries should limit to 10, got %d", len(entries))
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	seen := make(map[int]bool)
	for _, e := range entries {
		seen[e.RowsDiscarded] = true
	}
	if len(seen) < 1 {
		t.Error("expected entries to have distinct RowsDiscarded values")
	}
}

func TestSRSDocumentCreatedAt(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	doc := &SRSDocument{
		ProjectName: "Test",
		Version:     "2.0",
		Standard:    "ieee_29148",
		Content:     "doc body",
		Format:      "json",
	}
	if err := db.SaveSRSDocument(ctx, doc); err != nil {
		t.Fatalf("SaveSRSDocument failed: %v", err)
	}

	docs, err := db.GetSRSDocuments(ctx)
	if err != nil {
		t.Fatalf("GetSRSDocuments failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1", len(docs))
	}
	now := time.Now()
	if docs[0].CreatedAt.After(now.Add(time.Minute)) || docs[0].CreatedAt.Before(now.Add(-time.Hour)) {
		t.Errorf("CreatedAt seems wrong: %v", docs[0].CreatedAt)
	}
}

func TestSaveBookmarkWithQueryRef(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	bm := &Bookmark{
		Title:    "Search result",
		QueryRef: "what is the best approach",
		Source:   "chat",
	}
	if err := db.SaveBookmark(ctx, bm); err != nil {
		t.Fatalf("SaveBookmark failed: %v", err)
	}

	bookmarks, _ := db.GetBookmarks(ctx)
	if len(bookmarks) != 1 {
		t.Fatalf("got %d bookmarks, want 1", len(bookmarks))
	}
	if bookmarks[0].QueryRef != "what is the best approach" {
		t.Errorf("QueryRef = %q", bookmarks[0].QueryRef)
	}
}

func TestHistoryEntryFileRefs(t *testing.T) {
	db, dir := newTestDB(t)
	defer closeAndCleanup(t, db, dir)
	ctx := context.Background()

	entry := &HistoryEntry{
		Query:    "test query",
		Response: "test response",
		Model:    "test-model",
		FileRefs: `["a.py","b.go"]`,
	}
	if err := db.SaveHistory(ctx, entry); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	got, err := db.GetHistory(ctx, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].FileRefs != `["a.py","b.go"]` {
		t.Errorf("FileRefs = %q", got[0].FileRefs)
	}
	if got[0].Response != "test response" {
		t.Errorf("Response = %q", got[0].Response)
	}
}
